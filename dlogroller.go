package dlogroller

import (
	"errors"
	"fmt"
	"github.com/lestrrat-go/strftime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Roller struct {
	rootPath       string // rootPath 日志根路径，一切日志操作都基于此之上
	formatFileName string // formatFileName 格式化路径部分

	option
	writeLog

	startMill sync.Once
}

type option struct {
	maxSize int64 // 日志文件最大大小，单位兆
	maxAge  int   // 日志文件最大保存天数

	millEveryDayHour int // 每日N点开始执行陈旧文件处理，默认0点开始
}

type writeLog struct {
	patternFileName *strftime.Strftime // patternFileName 初始化Strftime对象
	nowOpenFileName string             // nowOpenFileName 当前打开的日志文件
	ext             string             // nowOpenFileName 当前打开的日志文件

	size int64      // 当前日志大小
	file *os.File   // file 当前日志写入对象
	mu   sync.Mutex // mu 互斥锁
}

// New dLogRoller
func New(rootPath, formatFileName string, options ...Option) (*Roller, error) {
	if rootPath == "" || formatFileName == "" {
		return nil, errors.New("rootPath或formatFileName 不可为空")
	}

	r := new(Roller)
	r.rootPath = rootPath
	r.formatFileName = path.Join(r.rootPath, formatFileName)

	for _, opt := range options {
		if err := opt.apply(r); err != nil {
			return nil, err
		}
	}

	_, err := r.mkdirAll()
	if err != nil {
		return nil, err
	}

	r.ext = filepath.Ext(r.nowOpenFileName)
	if r.ext == "" {
		return nil, errors.New("formatFileName 有误，无法识别后缀格式")
	}

	r.mill()
	return r, nil
}

func (r *Roller) mkdirAll() (os.FileMode, error) {
	if r.nowOpenFileName == "" {
		var err error
		r.patternFileName, err = strftime.New(r.formatFileName)
		if err != nil {
			return 0, errors.New("路径初始化失败")
		}
		r.nowOpenFileName = r.patternFileName.FormatString(time.Now())
	}

	info, err := os.Stat(r.nowOpenFileName)
	if err == nil {
		r.size = info.Size()
		return info.Mode(), nil
	}

	if os.IsNotExist(err) { // 目录不存在，创建
		err = os.MkdirAll(filepath.Dir(r.nowOpenFileName), 0755)
		if err != nil {
			return 0, fmt.Errorf("创建目录失败: %w", err)
		}
	}

	r.size = 0
	return 0644, nil
}

func (r *Roller) openFileName() error {
	if r.file == nil {
		mode, err := r.mkdirAll()
		if err != nil {
			return err
		}

		//O_RDONLY int = syscall.O_RDONLY // 只读模式打开文件
		//O_WRONLY int = syscall.O_WRONLY // 只写模式打开文件
		//O_RDWR   int = syscall.O_RDWR   // 读写模式打开文件
		//O_APPEND int = syscall.O_APPEND // 写操作时将数据附加到文件尾部
		//O_CREATE int = syscall.O_CREAT  // 如果不存在将创建一个新文件
		//O_EXCL   int = syscall.O_EXCL   // 和O_CREATE配合使用，文件必须不存在
		//O_SYNC   int = syscall.O_SYNC   // 打开文件用于同步I/O
		//O_TRUNC  int = syscall.O_TRUNC  // 如果可能，打开时清空文件
		r.file, err = os.OpenFile(r.nowOpenFileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, mode)
		if err != nil {
			return fmt.Errorf("日志文件打开失败: %w", err)
		}
	}

	return nil
}

func (r *Roller) rotate() error {
	if r.file == nil {
		return r.openFileName()
	}

	// 判断旋转前跟旋转后的目录是否一致，一致情况下直接返回nil
	nowOpenFileName := r.patternFileName.FormatString(time.Now())
	if r.nowOpenFileName == nowOpenFileName {
		return nil
	}

	err := r.file.Close()
	if err != nil {
		return err
	}

	r.nowOpenFileName = nowOpenFileName
	r.file = nil

	return r.openFileName()
}

// Write 写入
func (r *Roller) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.maxSize > 0 {
		r.size += int64(len(p))

		if r.size > r.maxSize { // 达到设定上限
			_ = r.rename()
		}
	}

	if err := r.rotate(); err != nil {
		return 0, err
	}

	return r.file.Write(p)
}

func (r *Roller) GetNowOpenFileName() string {
	return r.nowOpenFileName
}

func (r *Roller) rename() error {
	err := r.file.Close()
	if err != nil {
		return err
	}
	r.file = nil

	filename := filepath.Base(r.nowOpenFileName)
	if err = os.Rename(
		r.nowOpenFileName,
		filepath.Join(
			filepath.Dir(r.nowOpenFileName),
			filename[:len(filename)-len(r.ext)]+
				"_"+
				time.Now().Format("20060102T150405.000")+
				r.ext,
		),
	); err != nil {
		return fmt.Errorf("重命名失败: %w", err)
	}
	r.nowOpenFileName = ""

	return nil
}

// mill
func (r *Roller) mill() {
	r.startMill.Do(func() {
		go r.millRun()
	})
}

// millRun
func (r *Roller) millRun() {
	for {
		time.Sleep(50 * time.Minute)
		if r.millEveryDayHour == time.Now().Hour() {
			_ = r.millRunOnce()
		}
	}
}

// millRunOnce
func (r *Roller) millRunOnce() error {
	if r.maxAge == 0 {
		return nil
	}

	files, err := r.oldLogFiles()
	if err != nil {
		return err
	}

	if r.maxAge > 0 {
		cutoff := time.Now().AddDate(0, 0, -r.maxAge)

		for _, f := range files {
			if f.timestamp.Before(cutoff) {
				errRemove := os.Remove(f.fileName)
				if err == nil && errRemove != nil {
					err = errRemove
				}
			}
		}
	}

	return err
}

// oldLogFiles returns the list of backup log files stored in the same
// directory as the current log file, sorted by ModTime
func (r *Roller) oldLogFiles() ([]logInfo, error) {
	var logFiles []logInfo

	//获取指定目录下的所有文件或目录信息
	err := filepath.Walk(r.rootPath, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), r.ext) {
			return nil
		}

		logFiles = append(logFiles, logInfo{
			timestamp: info.ModTime(),
			fileName:  path,
			FileInfo:  info,
		})
		return nil
	})

	if err != nil && len(logFiles) == 0 {
		return nil, fmt.Errorf("读取目录失败：%w", err)
	}

	sort.Sort(byFormatTime(logFiles))

	return logFiles, nil
}

// logInfo is a convenience struct to return the filename and its embedded
// timestamp.
type logInfo struct {
	timestamp time.Time
	fileName  string
	os.FileInfo
}

// byFormatTime sorts by newest time formatted in the name.
type byFormatTime []logInfo

func (b byFormatTime) Less(i, j int) bool {
	return b[i].timestamp.After(b[j].timestamp)
}

func (b byFormatTime) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b byFormatTime) Len() int {
	return len(b)
}
