package archiver

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/klauspost/compress/zstd"
	"backup-go/internal/logger"
)

// CalculateDirSize 计算目录总大小（仅统计常规文件）
func CalculateDirSize(dir string) (int64, error) {
	var size int64
	visited := make(map[string]bool)

	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 检查符号链接循环
		if d.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(p)
			if err != nil {
				return nil // 跳过无法读取的符号链接
			}

			targetPath := filepath.Join(filepath.Dir(p), link)
			absTarget, err := filepath.Abs(targetPath)
			if err != nil {
				return nil // 跳过路径解析失败的符号链接
			}

			// 如果目标路径已经访问过，跳过以避免循环
			if visited[absTarget] {
				return nil // 跳过循环符号链接
			}
		}

		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}

		// 标记当前路径为已访问
		absPath, err := filepath.Abs(p)
		if err == nil {
			visited[absPath] = true
		}

		return nil
	})
	return size, err
}

// addTarEntry 写入一个条目到 tar
func addTarEntry(tw *tar.Writer, root, path string, d fs.DirEntry) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("计算相对路径失败: %w", err)
	}
	// 跳过根目录自身的条目 "."
	if rel == "." {
		return nil
	}
	name := filepath.ToSlash(rel)

	info, err := d.Info()
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %w", err)
	}

	// 符号链接：保留为链接条目（不解引用）
	if d.Type()&os.ModeSymlink != 0 {
		link, err := os.Readlink(path)
		if err != nil {
			logger.PrintLog("warn", fmt.Sprintf("跳过无法读取的符号链接: %s (错误: %v)", path, err))
			return nil // 跳过损坏的符号链接
		}

		// 安全检查：验证符号链接目标，防止路径遍历攻击
		if strings.Contains(link, "..") {
			logger.PrintLog("warn", fmt.Sprintf("跳过潜在不安全的符号链接: %s → %s (包含 '..')", path, link))
			return nil // 跳过而不是报错
		}

		// 检查符号链接目标是否是绝对路径
		if filepath.IsAbs(link) {
			logger.PrintLog("warn", fmt.Sprintf("跳过绝对路径符号链接: %s → %s", path, link))
			return nil // 跳过绝对路径链接
		}

		// 额外检查：验证相对路径符号链接的目标是否存在
		targetPath := filepath.Join(filepath.Dir(path), link)
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			logger.PrintLog("warn", fmt.Sprintf("跳过指向不存在目标的符号链接: %s → %s", path, link))
			return nil // 跳过死链接
		}

		h, err := tar.FileInfoHeader(info, filepath.ToSlash(link))
		if err != nil {
			logger.PrintLog("warn", fmt.Sprintf("跳过无法创建 tar 头的符号链接: %s (错误: %v)", path, err))
			return nil // 跳过处理失败的符号链接
		}
		h.Name = name
		if err := tw.WriteHeader(h); err != nil {
			return fmt.Errorf("写入符号链接 tar 头失败: %w", err)
		}
		return nil
	}

	// 目录：统一以 / 结尾
	if d.IsDir() {
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("创建 tar header 失败: %w", err)
		}
		if !strings.HasSuffix(name, "/") {
			name += "/"
		}
		h.Name = name
		return tw.WriteHeader(h)
	}

	// 常规文件
	if info.Mode().IsRegular() {
		h := &tar.Header{
			Name:    name,
			Size:    info.Size(),
			Mode:    int64(info.Mode().Perm()),
			ModTime: info.ModTime(),
		}
		if err := tw.WriteHeader(h); err != nil {
			return fmt.Errorf("写入 tar header 失败: %w", err)
		}
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("打开文件失败: %w", err)
		}
		if _, err := io.Copy(tw, f); err != nil {
			f.Close()
			return fmt.Errorf("拷贝文件内容失败: %w", err)
		}
		if closeErr := f.Close(); closeErr != nil {
			logger.PrintLog("warn", fmt.Sprintf("关闭文件失败: %s: %v", path, closeErr))
		}
		return nil
	}

	// 其他类型
	h, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("创建 tar header 失败: %w", err)
	}
	h.Name = name
	return tw.WriteHeader(h)
}

// Compress 压缩 data 目录为 zstd 压缩的 tar 包
func Compress(srcDir, dstFile string) (int64, int64, error) {
	logger.PrintLog("backup", "开始计算源目录大小: "+srcDir)
	originalSize, err := CalculateDirSize(srcDir)
	if err != nil {
		return 0, 0, fmt.Errorf("计算源目录大小失败: %w", err)
	}
	if originalSize == 0 {
		return 0, 0, fmt.Errorf("源目录 %s 为空，跳过备份", srcDir)
	}
	logger.PrintLog("backup", fmt.Sprintf("源目录大小: %s (%d bytes)", humanize.Bytes(uint64(originalSize)), originalSize))

	dst, err := os.Create(dstFile)
	if err != nil {
		return 0, 0, fmt.Errorf("创建压缩目标文件失败: %w", err)
	}
	
	defer func() {
		_ = dst.Close()
		if err != nil {
			_ = os.Remove(dstFile)
		}
	}()

	zs, err := zstd.NewWriter(dst, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return 0, 0, fmt.Errorf("创建 zstd 压缩器失败: %w", err)
	}
	tw := tar.NewWriter(zs)

	logger.PrintLog("backup", fmt.Sprintf("开始压缩打包 (zstd) %s → %s", srcDir, dstFile))

	var processedFiles, skippedFiles int64
	visited := make(map[string]bool)

	if err := filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			logger.PrintLog("warn", fmt.Sprintf("跳过文件访问错误: %s (错误: %v)", p, err))
			skippedFiles++
			return nil
		}

		// 检查符号链接循环
		if d.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(p)
			if err != nil {
				logger.PrintLog("warn", fmt.Sprintf("跳过无法读取的符号链接: %s (错误: %v)", p, err))
				skippedFiles++
				return nil
			}

			targetPath := filepath.Join(filepath.Dir(p), link)
			absTarget, err := filepath.Abs(targetPath)
			if err != nil {
				logger.PrintLog("warn", fmt.Sprintf("跳过路径解析失败的符号链接: %s → %s", p, link))
				skippedFiles++
				return nil
			}

			if visited[absTarget] {
				logger.PrintLog("warn", fmt.Sprintf("检测到循环符号链接，跳过: %s → %s", p, link))
				skippedFiles++
				return nil
			}
		}

		absPath, err := filepath.Abs(p)
		if err != nil {
			logger.PrintLog("warn", fmt.Sprintf("跳过路径解析失败的文件: %s", p))
			skippedFiles++
			return nil
		}
		visited[absPath] = true

		err = addTarEntry(tw, srcDir, p, d)
		if err != nil {
			logger.PrintLog("warn", fmt.Sprintf("跳过文件处理错误: %s (错误: %v)", p, err))
			skippedFiles++
			return nil
		}

		processedFiles++
		return nil
	}); err != nil {
		_ = tw.Close()
		_ = zs.Close()
		return 0, 0, fmt.Errorf("遍历并打包目录失败: %w", err)
	}

	logger.PrintLog("backup", fmt.Sprintf("文件处理统计: 成功 %d 个，跳过 %d 个", processedFiles, skippedFiles))
	if skippedFiles > 0 {
		logger.PrintLog("warn", fmt.Sprintf("备份过程中跳过了 %d 个有问题的文件，请检查上述警告信息", skippedFiles))
	}

	if err := tw.Close(); err != nil {
		_ = zs.Close()
		return 0, 0, fmt.Errorf("关闭 tar 写入器失败: %w", err)
	}
	if err := zs.Close(); err != nil {
		return 0, 0, fmt.Errorf("关闭 zstd 压缩器失败: %w", err)
	}

	info, err := os.Stat(dstFile)
	if err != nil {
		return 0, 0, fmt.Errorf("获取压缩文件大小失败: %w", err)
	}
	compressedSize := info.Size()

	logger.PrintLog("backup", "压缩完成")
	logger.PrintLog("backup", "原始大小: "+humanize.Bytes(uint64(originalSize)))
	logger.PrintLog("backup", "压缩后大小: "+humanize.Bytes(uint64(compressedSize)))
	if originalSize > 0 {
		logger.PrintLog("backup", fmt.Sprintf("压缩率: %.2f%%", float64(compressedSize)/float64(originalSize)*100))
	}
	return originalSize, compressedSize, nil
}
