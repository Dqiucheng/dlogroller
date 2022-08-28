package dlogroller

// An Option configures a Roller.
type Option interface {
	apply(*Roller) error
}

// optionFunc wraps a func so it satisfies the Option interface.
type optionFunc func(*Roller) error

func (f optionFunc) apply(log *Roller) error {
	return f(log)
}

// SetMaxSize 日志文件最大大小，单位兆
func SetMaxSize(maxSize int64) Option {
	return optionFunc(func(r *Roller) error {
		r.maxSize = maxSize * 1024 * 1024
		return nil
	})
}

// SetMaxAge 日志文件最大保存天数，0永久保存。
// 如果设置，不建议设置太多，避免一次性处理的文件太多造成卡顿
func SetMaxAge(maxAge int) Option {
	return optionFunc(func(r *Roller) error {
		r.maxAge = maxAge
		return nil
	})
}

// SetMillEveryDayHour 每日N点开始执行陈旧文件处理。
// 陈旧文件含义建立在maxAge参数之上，当maxAge为0时不会触发。
// 文件处理以日志根路径为基础
func SetMillEveryDayHour(hour int) Option {
	return optionFunc(func(r *Roller) error {
		r.millEveryDayHour = hour
		return nil
	})
}
