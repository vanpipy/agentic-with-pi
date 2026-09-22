package llm

import (
	"io"
	"log/slog"
	"sync"

	"github.com/vanpiyp/awp/internal/log"
)

var (
	ifaceLogOnce sync.Once
	ifaceLog     *slog.Logger
)

func ifaceLogger() *slog.Logger {
	ifaceLogOnce.Do(func() {
		l, err := log.NewFileLogger(log.LLMLogPath(), slog.LevelDebug)
		if err != nil || l == nil {
			ifaceLog = slog.New(slog.NewTextHandler(io.Discard, nil))
			return
		}
		ifaceLog = l
	})
	return ifaceLog
}

func resetIfaceLoggerForTest() {
	ifaceLogOnce = sync.Once{}
	ifaceLog = nil
}

func ResetIfaceLoggerForTest() {
	resetIfaceLoggerForTest()
}