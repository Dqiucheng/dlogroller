# dlogroller

### 这是一个高性能的日志插件，负责将日志进行分割写入，支持按时间、按大小进行分割。
### 下载
```
go get github.com/Dqiucheng/dlogroller
```

**示例**

```go
hook, err := dlogroller.New(
    path.Join("logs", "%Y%m", "%d", "%m-%dT%Haa.log"),  // 路径，文件名%H便实现了按每小时分割一次日志
    10,                                                 // 日志文件最大大小，单位兆，0不限制大小
)
```

**结合zap**

```go
logger := zap.New(
    zapcore.NewCore(
        encoderConfig,                                      // 编码器配置
        zapcore.NewMultiWriteSyncer(zapcore.AddSync(hook)), // 输入方式
        Level,                                              // 日志级别
    ),
)
```