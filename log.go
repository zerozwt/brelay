package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/zerozwt/BLiveDanmaku"
	"github.com/zerozwt/toyframe"
)

var g_logger atomic.Value

func init() {
	SetLogWriter(os.Stdout)
}

func logger() *log.Logger {
	return g_logger.Load().(*log.Logger)
}

func SetLogWriter(out io.Writer) {
	if out == nil {
		out = os.Stdout
	}
	logger := log.New(out, "brelay ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	g_logger.Store(logger)
	toyframe.SetLogWriter(out)
	BLiveDanmaku.SetLogWriter(out)
}

func dailyFileLogWriter(log_file string) io.Writer {
	if len(log_file) == 0 {
		return nil
	}

	tmp, err := os.OpenFile(log_file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil
	}
	tmp.Close()

	return &dailyLogWriter{log_file: log_file}
}

type dailyLogWriter struct {
	log_file string
}

func (w *dailyLogWriter) Write(b []byte) (int, error) {
	now := time.Now()
	suffix := fmt.Sprintf(".%04d%02d%02d", now.Year(), now.Month(), now.Day())
	tmp, err := os.OpenFile(w.log_file+suffix, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, err
	}
	defer tmp.Close()
	return tmp.Write(b)
}
