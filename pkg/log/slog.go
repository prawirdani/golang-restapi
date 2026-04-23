package log

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"
)

// ===================== Slog Adapter =====================
type SlogAdapter struct {
	l *slog.Logger
}

func NewSlogAdapter(production bool) *SlogAdapter {
	level := slog.LevelDebug
	if production {
		level = slog.LevelInfo
	}

	handlerOpts := &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.MessageKey {
				a.Key = "message"
			}
			if a.Key == slog.LevelKey {
				a.Value = slog.StringValue(strings.ToLower(a.Value.String()))
			}
			if a.Key == slog.TimeKey {
				a.Key = "timestamp"
			}
			if a.Key == slog.SourceKey {
				if source, ok := a.Value.Any().(*slog.Source); ok {
					a.Value = slog.StringValue(fmt.Sprintf("%s:%d", source.File, source.Line))
				}
			}
			return a
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, handlerOpts))
	if production {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, handlerOpts))
	}

	return &SlogAdapter{l: logger}
}

// Addtional 2 skips to capture the correct caller frame:
// Frame 3: this function
// Frame 4: wrapper (InfoCtx, DebugCtx ...)

func (s *SlogAdapter) Debug(msg string, args ...any) {
	s.logWithSkip(context.Background(), slog.LevelDebug, 2, msg, args...)
}

func (s *SlogAdapter) Info(msg string, args ...any) {
	s.logWithSkip(context.Background(), slog.LevelInfo, 2, msg, args...)
}

func (s *SlogAdapter) Warn(msg string, args ...any) {
	s.logWithSkip(context.Background(), slog.LevelWarn, 2, msg, args...)
}

func (s *SlogAdapter) Error(msg string, err error, args ...any) {
	args = append(args, "error", err.Error())
	s.logWithSkip(context.Background(), slog.LevelError, 2, msg, args...)
}

func (s *SlogAdapter) DebugCtx(ctx context.Context, msg string, args ...any) {
	s.buildContextualLogger(ctx, slog.LevelDebug, msg, args...)
}

func (s *SlogAdapter) InfoCtx(ctx context.Context, msg string, args ...any) {
	s.buildContextualLogger(ctx, slog.LevelInfo, msg, args...)
}

func (s *SlogAdapter) WarnCtx(ctx context.Context, msg string, args ...any) {
	s.buildContextualLogger(ctx, slog.LevelWarn, msg, args...)
}

func (s *SlogAdapter) ErrorCtx(ctx context.Context, msg string, err error, args ...any) {
	args = append(args, "error", err.Error())
	s.buildContextualLogger(ctx, slog.LevelError, msg, args...)
}

func (s *SlogAdapter) With(args ...any) Logger {
	return &SlogAdapter{
		l: s.l.With(normalizeSlogArgs(args)...),
	}
}

func (s *SlogAdapter) logWithSkip(
	ctx context.Context,
	level slog.Level,
	skip int,
	msg string,
	args ...any,
) {
	if !s.l.Enabled(ctx, level) {
		return
	}

	var pcs [1]uintptr
	// Frames:
	// 1 runtime.Callers
	// 2 logWithSkip
	// + skip
	runtime.Callers(skip+2, pcs[:])

	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(normalizeSlogArgs(args)...)

	_ = s.l.Handler().Handle(ctx, r)
}

func (s *SlogAdapter) buildContextualLogger(
	ctx context.Context,
	level slog.Level,
	msg string,
	args ...any,
) {
	if l := GetFromContext(ctx); l != nil {
		if sa, ok := l.(*SlogAdapter); ok {
			// Frames:
			// 3 buildContextualLogger
			// 4 wrapper (InfoCtx, DebugCtx, ...)
			// 5 actual caller
			sa.logWithSkip(ctx, level, 3, msg, args...)
			return
		}
	}

	s.logWithSkip(ctx, level, 3, msg, args...)
}

func normalizeSlogArgs(args []any) []any {
	out := make([]any, 0, len(args))

	for i := 0; i < len(args); {
		switch v := args[i].(type) {

		// -------- nested group --------
		case group:
			out = append(out, slog.Group(v.Key, normalizeSlogArgs(v.Attrs)...))
			i++

		// -------- slog native --------
		case slog.Attr:
			out = append(out, v)
			i++

		// -------- flat key-value --------
		case string:
			if i+1 < len(args) {
				out = append(out, slog.Any(v, args[i+1]))
				i += 2
			} else {
				i++
			}

		// -------- unsupported --------
		default:
			out = append(out, slog.Any("value", v))
			i++
		}
	}

	return out
}
