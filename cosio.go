package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/tencentyun/cos-go-sdk-v5"
)

func createCOSClient(cfg *Config) (*cos.Client, error) {
	bu, err := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", cfg.Cos.Bucket, cfg.Cos.Region))
	if err != nil {
		return nil, fmt.Errorf("构造 Bucket URL 失败: %w", err)
	}
	su, err := url.Parse(fmt.Sprintf("https://cos.%s.myqcloud.com", cfg.Cos.Region))
	if err != nil {
		return nil, fmt.Errorf("构造 Service URL 失败: %w", err)
	}
	// 加强 HTTP 连接稳健性（不设置 http.Client.Timeout，以免大文件上传被整体超时截断）
	baseTransport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}
	return cos.NewClient(
		&cos.BaseURL{BucketURL: bu, ServiceURL: su},
		&http.Client{Transport: &cos.AuthorizationTransport{
			SecretID:  cfg.Cos.SecretID,
			SecretKey: cfg.Cos.SecretKey,
			Transport: baseTransport,
		}},
	), nil
}

func testCOSConnection(client *cos.Client, bucket string) error {
	printLog("check", "正在测试 COS 连接...")
	_, _, err := client.Bucket.Get(context.Background(), &cos.BucketGetOptions{})
	if err != nil {
		return fmt.Errorf("COS 连接测试失败: %w", err)
	}
	printLog("check", "COS 连接成功")
	return nil
}

// ---- 上传进度（优化：可配置刷新周期，避免 100% 重复打印）----

func friendlyDuration(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	d = d.Round(time.Second)
	if d >= time.Hour {
		h := d / time.Hour
		m := (d % time.Hour) / time.Minute
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if d >= time.Minute {
		m := d / time.Minute
		s := (d % time.Minute) / time.Second
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

type progressReader struct {
	r        io.Reader
	total    int64
	read     int64
	lastTime time.Time
	lastRead int64
	interval time.Duration
	onTick   func(read, total, rate int64, eta time.Duration)
	finished bool
}

func newProgressReader(r io.Reader, total int64, interval time.Duration, onTick func(read, total, rate int64, eta time.Duration)) *progressReader {
	return &progressReader{
		r:        r,
		total:    total,
		interval: interval,
		onTick:   onTick,
	}
}

func (p *progressReader) Read(b []byte) (int, error) {
	if p.finished {
		return 0, io.EOF
	}
	n, err := p.r.Read(b)
	if n > 0 {
		p.read += int64(n)
	}
	now := time.Now()

	needTick := false
	if p.lastTime.IsZero() || now.Sub(p.lastTime) >= p.interval {
		needTick = true
	}
	// 确保 EOF 时只打印一次最终 100%
	if err == io.EOF && p.read == p.total && !p.finished {
		needTick = true
		p.finished = true
	}

	if needTick {
		dur := now.Sub(p.lastTime)
		if p.lastTime.IsZero() || dur <= 0 {
			dur = time.Second
		}
		rate := (p.read - p.lastRead) * int64(time.Second) / int64(dur) // bytes/s
		var eta time.Duration
		if p.total > 0 && rate > 0 {
			eta = time.Duration((p.total - p.read) * int64(time.Second) / rate)
		}
		if p.onTick != nil {
			p.onTick(p.read, p.total, rate, eta)
		}
		p.lastTime = now
		p.lastRead = p.read
	}
	return n, err
}

func uploadToCOS(client *cos.Client, localFile, cosPath string) error {
	fi, err := os.Stat(localFile)
	if err != nil {
		return fmt.Errorf("获取本地文件信息失败: %w", err)
	}
	printLog("upload", fmt.Sprintf("开始上传文件: %s → %s", localFile, cosPath))

	// 允许通过环境变量调整进度刷新间隔（默认 1s），示例：PROGRESS_INTERVAL=500ms
	interval := time.Second
	if v := os.Getenv("PROGRESS_INTERVAL"); v != "" {
		// 安全验证：限制环境变量长度和字符
		if len(v) > 20 { // 限制长度防止恶意输入
			printLog("warn", "环境变量 PROGRESS_INTERVAL 过长，使用默认值")
		} else if d, e := time.ParseDuration(v); e == nil && d > 0 && d <= 60*time.Second {
			// 限制范围：1ms 到 60秒
			interval = d
		} else {
			printLog("warn", "环境变量 PROGRESS_INTERVAL 格式无效或超出范围(1ms-60s)，使用默认值")
		}
	}

	// 简单重试：最多 3 次指数退避
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		f, err := os.Open(localFile)
		if err != nil {
			return fmt.Errorf("打开本地文件失败: %w", err)
		}

		pr := newProgressReader(f, fi.Size(), interval, func(read, total, rate int64, eta time.Duration) {
			percent := 0
			if total > 0 {
				percent = int(float64(read) / float64(total) * 100)
			}
			printLog("upload", fmt.Sprintf("进度: %3d%% (%s/%s) %s/s ETA %s",
				percent,
				humanize.Bytes(uint64(read)),
				humanize.Bytes(uint64(total)),
				humanize.Bytes(uint64(rate)),
				friendlyDuration(eta),
			))
		})

		_, err = client.Object.Put(context.Background(), cosPath, pr, &cos.ObjectPutOptions{
			ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{ContentLength: fi.Size()},
		})
		_ = f.Close()

		if err == nil {
			printLog("upload", "文件上传完成")
			return nil
		}

		lastErr = err
		if attempt < 3 {
			backoff := time.Duration(1<<uint(attempt-1)) * 500 * time.Millisecond
			printLog("warn", fmt.Sprintf("上传失败，准备重试（第 %d/3 次，%s 后重试）：%v", attempt, backoff, err))
			time.Sleep(backoff)
		}
	}
	return fmt.Errorf("上传文件到 COS 失败（已重试 3 次）：%w", lastErr)
}

// ---- 过期清理 ----

func isBackupObject(key string) bool {
	name := filepath.Base(key)
	return strings.HasPrefix(name, "backup-") && strings.HasSuffix(name, ".tar.zst")
}

func parseBackupTime(key string) (time.Time, bool) {
	name := filepath.Base(key)
	ts := strings.TrimSuffix(strings.TrimPrefix(name, "backup-"), ".tar.zst")
	t, err := time.Parse("20060102-150405", ts)
	return t, err == nil
}

// 并发删除 + 分页列举；统计区分“总列举/有效对象/匹配命名/待删除/实际删除”
func deleteExpiredBackups(client *cos.Client, _ string, cosBasePath string, keepDays int) error {
	if keepDays <= 0 {
		printLog("info", "保留天数为 0 或负数，跳过过期文件清理")
		return nil
	}
	printLog("cleanup", "开始清理过期备份文件...")
	expire := time.Now().AddDate(0, 0, -keepDays)
	printLog("cleanup", fmt.Sprintf("删除 %s 之前创建的备份文件", expire.Format("2006-01-02")))

	const workers = 5
	type task struct{ key string }
	tasks := make(chan task, 256)

	var wg sync.WaitGroup
	var deleted, failed int64
	var mu sync.Mutex

	workerFn := func() {
		defer wg.Done()
		for t := range tasks {
			_, err := client.Object.Delete(context.Background(), t.key)
			if err != nil {
				printLog("error", fmt.Sprintf("删除 COS 对象失败: %s: %v", t.key, err))
				mu.Lock()
				failed++
				mu.Unlock()
				continue
			}
			mu.Lock()
			deleted++
			mu.Unlock()
			printLog("cleanup", "已删除: "+t.key)
		}
	}
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go workerFn()
	}

	listedRaw := 0   // 前缀下总返回数（含占位对象）
	listedFiles := 0 // 过滤掉以 / 结尾的占位符后的有效对象数
	matched := 0     // 符合 backup-*.tar.zst 命名的数
	toDelete := 0    // 需要删除的数量

	marker := ""
	for {
		v, _, err := client.Bucket.Get(context.Background(), &cos.BucketGetOptions{
			Prefix:  cosBasePath,
			Marker:  marker,
			MaxKeys: 1000,
		})
		if err != nil {
			close(tasks)
			wg.Wait()
			return fmt.Errorf("列举 COS 对象失败: %w", err)
		}

		for _, it := range v.Contents {
			listedRaw++
			// 跳过目录占位对象（一般 key 以 / 结尾）
			if strings.HasSuffix(it.Key, "/") {
				continue
			}
			listedFiles++

			if !isBackupObject(it.Key) {
				continue
			}
			matched++

			if ts, ok := parseBackupTime(it.Key); ok && ts.Before(expire) {
				toDelete++
				tasks <- task{key: it.Key}
			}
		}

		if !v.IsTruncated {
			break
		}
		marker = v.NextMarker
	}

	close(tasks)
	wg.Wait()

	printLog("cleanup", fmt.Sprintf(
		"列举总数 %d 个（有效对象 %d 个），其中匹配备份命名 %d 个，待删除 %d 个；实际删除 %d 个，失败 %d 个",
		listedRaw, listedFiles, matched, toDelete, deleted, failed,
	))
	return nil
}
