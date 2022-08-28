package dlogroller

import (
	"errors"
	"fmt"
	"github.com/lestrrat-go/strftime"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Roller struct {
	fileName        string             // fileName 日志目录文件
	patternFileName *strftime.Strftime // patternFileName 初始化Strftime对象
	nowOpenFileName string             // nowOpenFileName 当前打开的日志文件

	maxSize int64      // 日志文件最大大小，单位兆
	size    int64      // 日志大小
	file    *os.File   // file 日志写入对象
	mu      sync.Mutex // mu 互斥锁

	startMill sync.Once
}

func New(fileName string, maxSize int64) (*Roller, error) {
	r := new(Roller)
	r.fileName = fileName
	r.maxSize = maxSize * 1024 * 1024

	if r.fileName == "" {
		return nil, errors.New("filename 不可为空")
	}

	_, err := r.mkdirAll()
	if err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Roller) mkdirAll() (os.FileMode, error) {
	if r.nowOpenFileName == "" {
		var err error
		r.patternFileName, err = strftime.New(r.fileName)
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

func (r *Roller) rename() error {
	err := r.file.Close()
	if err != nil {
		return err
	}
	r.file = nil

	filename := filepath.Base(r.nowOpenFileName)
	ext := filepath.Ext(filename)

	if err = os.Rename(
		r.nowOpenFileName,
		filepath.Join(
			filepath.Dir(r.nowOpenFileName),
			filename[:len(filename)-len(ext)]+
				"_"+
				time.Now().Format("20060102T150405.000")+
				ext,
		),
	); err != nil {
		return fmt.Errorf("重命名失败: %w", err)
	}
	r.nowOpenFileName = ""

	return nil
}
