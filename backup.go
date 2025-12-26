package main

import (
	"archive/tar"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/fsnotify/fsnotify"
	"github.com/klauspost/compress/zstd"
	"github.com/tencentyun/cos-go-sdk-v5"
)

// 计算目录总大小（仅统计常规文件）
func calculateDirSize(dir string) (int64, error) {
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

// 写入一个条目到 tar（默认不跟随符号链接；普通文件仅保留权限位 Perm）
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
			printLog("warn", fmt.Sprintf("跳过无法读取的符号链接: %s (错误: %v)", path, err))
			return nil // 跳过损坏的符号链接
		}

		// 安全检查：验证符号链接目标，防止路径遍历攻击
		if strings.Contains(link, "..") {
			printLog("warn", fmt.Sprintf("跳过潜在不安全的符号链接: %s → %s (包含 '..')", path, link))
			return nil // 跳过而不是报错，保持备份流程继续
		}

		// 检查符号链接目标是否是绝对路径
		if filepath.IsAbs(link) {
			printLog("warn", fmt.Sprintf("跳过绝对路径符号链接: %s → %s", path, link))
			return nil // 跳过绝对路径链接
		}

		// 额外检查：验证相对路径符号链接的目标是否存在
		targetPath := filepath.Join(filepath.Dir(path), link)
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			printLog("warn", fmt.Sprintf("跳过指向不存在目标的符号链接: %s → %s", path, link))
			return nil // 跳过死链接
		}

		h, err := tar.FileInfoHeader(info, filepath.ToSlash(link))
		if err != nil {
			printLog("warn", fmt.Sprintf("跳过无法创建 tar 头的符号链接: %s (错误: %v)", path, err))
			return nil // 跳过处理失败的符号链接
		}
		h.Name = name
		if err := tw.WriteHeader(h); err != nil {
			return fmt.Errorf("写入符号链接 tar 头失败: %w", err)
		}
		return nil
	}

	// 目录：统一以 / 结尾，兼容性更好
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

	// 常规文件：仅保留权限位（与原逻辑一致）
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
			f.Close() // 立即关闭文件
			return fmt.Errorf("拷贝文件内容失败: %w", err)
		}
		if closeErr := f.Close(); closeErr != nil {
			printLog("warn", fmt.Sprintf("关闭文件失败: %s: %v", path, closeErr))
		}
		return nil
	}

	// 其他类型：尽力写入 header（FIFO/设备等，通常很少见）
	h, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("创建 tar header 失败: %w", err)
	}
	h.Name = name
	return tw.WriteHeader(h)
}

// 压缩 data 目录为 zstd 压缩的 tar 包（先关闭 tar/zstd 再统计大小）
func compressData(srcDir, dstFile string) (int64, int64, error) {
	printLog("backup", "开始计算源目录大小: "+srcDir)
	originalSize, err := calculateDirSize(srcDir)
	if err != nil {
		return 0, 0, fmt.Errorf("计算源目录大小失败: %w", err)
	}
	if originalSize == 0 {
		return 0, 0, fmt.Errorf("源目录 %s 为空，跳过备份", srcDir)
	}
	printLog("backup", fmt.Sprintf("源目录大小: %s (%d bytes)", humanize.Bytes(uint64(originalSize)), originalSize))

	dst, err := os.Create(dstFile)
	if err != nil {
		return 0, 0, fmt.Errorf("创建压缩目标文件失败: %w", err)
	}
	// 注意：不要提前 defer 关闭 writer，确保先 Close 再 Stat
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

	printLog("backup", fmt.Sprintf("开始压缩打包 (zstd) %s → %s", srcDir, dstFile))

	// 统计信息
	var processedFiles, skippedFiles int64

	// 安全的目录遍历，包含循环符号链接检测
	visited := make(map[string]bool)
	if err := filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			printLog("warn", fmt.Sprintf("跳过文件访问错误: %s (错误: %v)", p, err))
			skippedFiles++
			return nil // 跳过有问题的文件，继续处理其他文件
		}

		// 检查符号链接循环
		if d.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(p)
			if err != nil {
				printLog("warn", fmt.Sprintf("跳过无法读取的符号链接: %s (错误: %v)", p, err))
				skippedFiles++
				return nil // 跳过无法读取的符号链接
			}

			targetPath := filepath.Join(filepath.Dir(p), link)
			absTarget, err := filepath.Abs(targetPath)
			if err != nil {
				printLog("warn", fmt.Sprintf("跳过路径解析失败的符号链接: %s → %s", p, link))
				skippedFiles++
				return nil // 跳过路径解析失败的符号链接
			}

			// 如果目标路径已经访问过，跳过以避免循环
			if visited[absTarget] {
				printLog("warn", fmt.Sprintf("检测到循环符号链接，跳过: %s → %s", p, link))
				skippedFiles++
				return nil // 跳过循环符号链接
			}
		}

		// 标记当前路径为已访问
		absPath, err := filepath.Abs(p)
		if err != nil {
			printLog("warn", fmt.Sprintf("跳过路径解析失败的文件: %s", p))
			skippedFiles++
			return nil // 跳过路径解析失败的文件
		}
		visited[absPath] = true

		err = addTarEntry(tw, srcDir, p, d)
		if err != nil {
			printLog("warn", fmt.Sprintf("跳过文件处理错误: %s (错误: %v)", p, err))
			skippedFiles++
			return nil // 跳过处理失败的文件，继续备份其他文件
		}

		processedFiles++
		return nil
	}); err != nil {
		_ = tw.Close()
		_ = zs.Close()
		return 0, 0, fmt.Errorf("遍历并打包目录失败: %w", err)
	}

	// 输出统计信息
	printLog("backup", fmt.Sprintf("文件处理统计: 成功 %d 个，跳过 %d 个", processedFiles, skippedFiles))
	if skippedFiles > 0 {
		printLog("warn", fmt.Sprintf("备份过程中跳过了 %d 个有问题的文件，请检查上述警告信息", skippedFiles))
	}

	// 先关闭 tar writer 和 zstd writer，确保数据落盘
	if err := tw.Close(); err != nil {
		_ = zs.Close()
		return 0, 0, fmt.Errorf("关闭 tar 写入器失败: %w", err)
	}
	if err := zs.Close(); err != nil {
		return 0, 0, fmt.Errorf("关闭 zstd 压缩器失败: %w", err)
	}

	// 现在再统计压缩后的大小，避免统计到未刷新的尺寸
	info, err := os.Stat(dstFile)
	if err != nil {
		return 0, 0, fmt.Errorf("获取压缩文件大小失败: %w", err)
	}
	compressedSize := info.Size()

	printLog("backup", "压缩完成")
	printLog("backup", "原始大小: "+humanize.Bytes(uint64(originalSize)))
	printLog("backup", "压缩后大小: "+humanize.Bytes(uint64(compressedSize)))
	if originalSize > 0 {
		printLog("backup", fmt.Sprintf("压缩率: %.2f%%", float64(compressedSize)/float64(originalSize)*100))
	}
	return originalSize, compressedSize, nil
}

func generateArchivePath() string {
	return filepath.Join(getTempDir(), fmt.Sprintf("backup-%s.tar.zst", time.Now().Format("20060102-150405")))
}

func performBackup(cfg *Config, cosClient *cos.Client, archivePath string) error {
	printLog("backup", "开始备份...")
	_, _, err := compressData(cfg.Backup.DataDir, archivePath)
	if err != nil && strings.Contains(err.Error(), "为空，跳过备份") {
		printLog("skip", err.Error())
		_ = os.RemoveAll(getTempDir())
		os.Exit(0)
	}
	if err != nil {
		return fmt.Errorf("压缩失败: %w", err)
	}
	return uploadToCOS(cosClient, archivePath, cfg.Cos.Prefix+filepath.Base(archivePath))
}

func main() {
	// 定义命令行参数
	var generateScript bool
	var showHelp bool
	var showVersion bool
	var daemonMode bool

	flag.BoolVar(&generateScript, "init", false, "生成管理脚本和默认配置文件")
	flag.BoolVar(&showHelp, "help", false, "显示帮助信息")
	flag.BoolVar(&showVersion, "version", false, "显示版本信息")
	flag.BoolVar(&daemonMode, "daemon", false, "以守护进程模式运行")
	flag.Parse()

	// 处理命令行参数
	if showVersion {
		fmt.Println("backup-go v1.0.0")
		fmt.Println("腾讯云 COS 备份工具")
		fmt.Println("")
		fmt.Println("功能特性:")
		fmt.Println("  • 支持 Docker 环境容错备份")
		fmt.Println("  • ZSTD 高效压缩算法")
		fmt.Println("  • 腾讯云 COS 自动上传")
		fmt.Println("  • 定时任务和自动清理")
		fmt.Println("  • macOS launchd 集成")
		fmt.Println("")
		fmt.Println("使用方法:")
		fmt.Println("  ./backup-go                    # 执行备份")
		fmt.Println("  ./backup-go --init            # 生成管理脚本")
		fmt.Println("  ./backup-go --help            # 显示帮助")
		fmt.Println("  ./backup-go --version         # 显示版本")
		return
	}

	if showHelp {
		fmt.Println("backup-go - 腾讯云 COS 备份工具")
		fmt.Println("")
		fmt.Println("用法:")
		fmt.Println("  ./backup-go [选项]")
		fmt.Println("")
		fmt.Println("选项:")
		fmt.Println("  --init          生成管理脚本和默认配置文件")
		fmt.Println("  --help          显示帮助信息")
		fmt.Println("  --version       显示版本信息")
		fmt.Println("")
		fmt.Println("示例:")
		fmt.Println("  ./backup-go --init")
		fmt.Println("  ./backup-service.sh install")
		fmt.Println("  ./backup-service.sh start")
		return
	}

	// 处理守护进程模式
	if daemonMode {
		// 初始化基础设置
		exitIfError(setupDirectories(), "确保目录存在失败")

		// 检查配置文件，如果不存在则生成
		cfgPath := filepath.Join(ConfigDir, ConfigFile)
		if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
			exitIfError(generateDefaultConfig(cfgPath), "生成默认配置文件失败")
		}

		// 加载配置
		cfg, err := loadConfig(cfgPath)
		exitIfError(err, "加载配置失败")

		// 创建COS客户端
		cosClient, err := createCOSClient(cfg)
		exitIfError(err, "创建COS客户端失败")

		// 启动守护进程模式
		startDaemonMode(cfg, cosClient)
		return
	}

	// 初始化基础设置
	exitIfError(setupDirectories(), "确保目录存在失败")

	// 检查配置文件，如果不存在则生成
	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		exitIfError(generateDefaultConfig(cfgPath), "生成默认配置文件失败")
		printLog("info", "已生成默认配置文件: "+cfgPath)
		printLog("info", "请编辑配置文件设置腾讯云 COS 认证信息")
		printLog("info", "现在将启动交互式菜单进行配置...")
		// 启动交互式菜单进行初始配置
		showInteractiveMenu()
		return
	}

	// 默认启动交互式菜单
	showInteractiveMenu()
}

// 执行一次性备份
func performOneTimeBackup(cfg *Config, cosClient *cos.Client) {
	exitIfError(prepareTempDir(), "准备临时目录失败")

	archivePath := generateArchivePath()
	if err := performBackup(cfg, cosClient, archivePath); err != nil {
		printLog("error", err.Error())
		_ = os.RemoveAll(getTempDir())
		os.Exit(1)
	}

	if err := deleteExpiredBackups(cosClient, cfg.Cos.Bucket, cfg.Cos.Prefix, cfg.Cos.KeepDays); err != nil {
		printLog("error", fmt.Sprintf("清理过期备份失败: %v", err))
	}

	exitIfError(cleanupTempDir(), "清理临时目录失败")
	printLog("done", "备份流程完成")
}

// 执行定时备份
// 检查是否有其他备份进程正在运行
func checkBackupRunning() bool {
	pidFile := filepath.Join(TempDir, "backup.pid")

	// 检查PID文件是否存在
	if _, err := os.Stat(pidFile); err == nil {
		// 读取PID文件
		data, err := os.ReadFile(pidFile)
		if err != nil {
			printLog("error", "读取PID文件失败")
			return false
		}

		// 解析PID
		var pid int
		if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil || pid <= 0 {
			printLog("error", "PID文件格式无效")
			_ = os.Remove(pidFile)
			return false
		}

		// 检查进程是否存在
		process, err := os.FindProcess(pid)
		if err != nil {
			printLog("error", "查找进程失败")
			_ = os.Remove(pidFile)
			return false
		}

		// 发送信号0检查进程是否还在运行
		if err := process.Signal(syscall.Signal(0)); err != nil {
			printLog("info", "发现残留的PID文件，清理中...")
			_ = os.Remove(pidFile)
			return false
		}

		// 进程仍在运行
		printLog("warn", fmt.Sprintf("检测到备份进程正在运行 (PID: %d)", pid))
		return true
	}

	return false
}

// 创建PID文件
func createPidFile() error {
	pidFile := filepath.Join(TempDir, "backup.pid")

	// 确保临时目录存在
	if err := os.MkdirAll(TempDir, 0755); err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}

	// 写入当前进程PID
	pid := os.Getpid()
	return os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), 0644)
}

// 删除PID文件
func removePidFile() {
	pidFile := filepath.Join(TempDir, "backup.pid")
	_ = os.Remove(pidFile)
}

// 执行定时备份（带并发控制）
func runScheduledBackup(cfg *Config, cosClient *cos.Client) error {
	// 检查是否有其他备份进程正在运行
	if checkBackupRunning() {
		printLog("warn", "定时备份跳过：已有备份进程正在运行")
		return nil
	}

	// 创建PID文件
	if err := createPidFile(); err != nil {
		printLog("error", fmt.Sprintf("创建PID文件失败: %v", err))
		return err
	}

	// 确保在函数结束时删除PID文件
	defer removePidFile()

	printLog("schedule", "=== 开始执行定时备份 ===")

	exitIfError(prepareTempDir(), "准备临时目录失败")

	archivePath := generateArchivePath()
	if err := performBackup(cfg, cosClient, archivePath); err != nil {
		printLog("error", fmt.Sprintf("定时备份失败: %v", err))
		_ = os.RemoveAll(getTempDir())
		return err
	}

	// 清理过期备份
	if err := deleteExpiredBackups(cosClient, cfg.Cos.Bucket, cfg.Cos.Prefix, cfg.Cos.KeepDays); err != nil {
		printLog("error", fmt.Sprintf("清理过期备份失败: %v", err))
	}

	exitIfError(cleanupTempDir(), "清理临时目录失败")
	printLog("schedule", "=== 定时备份完成 ===")
	return nil
}

// 守护进程模式 - 持续运行定时任务
func startDaemonMode(cfg *Config, cosClient *cos.Client) {
	printLog("daemon", "=== 启动守护进程模式 ===")
	printLog("daemon", fmt.Sprintf("定时任务配置: 每天 %02d:%02d 执行",
		cfg.Backup.Schedule.Hour, cfg.Backup.Schedule.Minute))
	printLog("daemon", fmt.Sprintf("时区: %s", cfg.Backup.Schedule.Timezone))
	printLog("daemon", "按 Ctrl+C 停止程序")
	printLog("daemon", "配置文件监控已启用，修改配置后将自动重载")

	// 创建配置文件监控器
	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		printLog("error", fmt.Sprintf("创建文件监控器失败: %v", err))
		return
	}
	defer watcher.Close()

	// 监控配置文件
	if err := watcher.Add(cfgPath); err != nil {
		printLog("error", fmt.Sprintf("监控配置文件失败: %v", err))
		return
	}

	// 信号处理，优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 创建配置重载通道
	configReloadChan := make(chan bool, 1)

	// 启动配置文件监控协程
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					printLog("daemon", "检测到配置文件变化，正在重载...")
					configReloadChan <- true
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				printLog("error", fmt.Sprintf("文件监控错误: %v", err))
			}
		}
	}()

	for {
		// 计算下次执行时间
		nextRunTime := calculateNextRunTime(cfg.Backup.Schedule)
		now := time.Now()

		if cfg.Backup.Schedule.Enabled {
			// 计算等待时间
			duration := nextRunTime.Sub(now)
			if duration > 0 {
				printLog("daemon", fmt.Sprintf("下次执行时间: %s (等待 %v)",
					nextRunTime.Format("2006-01-02 15:04:05"), duration))

				// 等待多个事件：定时时间、配置重载、停止信号
				select {
				case <-time.After(duration):
					// 时间到了，执行备份
					printLog("daemon", "开始执行定时备份...")
					if err := runScheduledBackup(cfg, cosClient); err != nil {
						printLog("error", fmt.Sprintf("定时备份执行失败: %v", err))
					}
				case <-configReloadChan:
					// 配置文件变化，重新加载配置
					if newCfg, err := loadConfig(cfgPath); err == nil {
						oldEnabled := cfg.Backup.Schedule.Enabled
						cfg = newCfg
						printLog("daemon", "配置重载成功")

						// 检查定时任务状态变化
						if !oldEnabled && cfg.Backup.Schedule.Enabled {
							printLog("daemon", "定时任务已启用")
						} else if oldEnabled && !cfg.Backup.Schedule.Enabled {
							printLog("daemon", "定时任务已禁用")
						} else {
							printLog("daemon", fmt.Sprintf("定时任务配置更新: 每天 %02d:%02d",
								cfg.Backup.Schedule.Hour, cfg.Backup.Schedule.Minute))
						}
					} else {
						printLog("error", fmt.Sprintf("配置重载失败: %v", err))
					}
				case <-sigChan:
					printLog("daemon", "收到停止信号，正在退出...")
					return
				}
			} else {
				// 如果当前时间已经过了执行时间，等待到明天
				tomorrow := now.AddDate(0, 0, 1)
				targetTime := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(),
					cfg.Backup.Schedule.Hour, cfg.Backup.Schedule.Minute, 0, 0,
					now.Location())

				printLog("daemon", fmt.Sprintf("今天执行时间已过，下次执行时间: %s",
					targetTime.Format("2006-01-02 15:04:05")))

				select {
				case <-time.After(targetTime.Sub(now)):
					printLog("daemon", "开始执行定时备份...")
					if err := runScheduledBackup(cfg, cosClient); err != nil {
						printLog("error", fmt.Sprintf("定时备份执行失败: %v", err))
					}
				case <-configReloadChan:
					// 配置文件变化，重新加载配置
					if newCfg, err := loadConfig(cfgPath); err == nil {
						oldEnabled := cfg.Backup.Schedule.Enabled
						cfg = newCfg
						printLog("daemon", "配置重载成功")

						// 检查定时任务状态变化
						if !oldEnabled && cfg.Backup.Schedule.Enabled {
							printLog("daemon", "定时任务已启用")
						} else if oldEnabled && !cfg.Backup.Schedule.Enabled {
							printLog("daemon", "定时任务已禁用")
						} else {
							printLog("daemon", fmt.Sprintf("定时任务配置更新: 每天 %02d:%02d",
								cfg.Backup.Schedule.Hour, cfg.Backup.Schedule.Minute))
						}
					} else {
						printLog("error", fmt.Sprintf("配置重载失败: %v", err))
					}
				case <-sigChan:
					printLog("daemon", "收到停止信号，正在退出...")
					return
				}
			}
		} else {
			// 定时任务未启用，等待配置变化或停止信号
			printLog("daemon", "定时任务未启用，等待配置文件变化...")
			select {
			case <-configReloadChan:
				// 配置文件变化，重新加载配置
				if newCfg, err := loadConfig(cfgPath); err == nil {
					oldEnabled := cfg.Backup.Schedule.Enabled
					cfg = newCfg
					printLog("daemon", "配置重载成功")

					// 检查定时任务状态变化
					if !oldEnabled && cfg.Backup.Schedule.Enabled {
						printLog("daemon", "检测到定时任务已启用")
					} else {
						printLog("daemon", "定时任务仍处于禁用状态")
					}
				} else {
					printLog("error", fmt.Sprintf("配置重载失败: %v", err))
				}
			case <-sigChan:
				printLog("daemon", "收到停止信号，正在退出...")
				return
			}
		}
	}
}
