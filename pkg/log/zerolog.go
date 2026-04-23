package log

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// ===================== Zerolog Adapter =====================

// ZerologAdapter adapts zerolog to the Logger interface
type ZerologAdapter struct {
	l zerolog.Logger
}

func NewZerologAdapter(production bool) *ZerologAdapter {
	// Inlining common field keys and format with slog version
	zerolog.TimestampFieldName = "timestamp"
	zerolog.CallerFieldName = "source"
	zerolog.TimeFieldFormat = time.RFC3339Nano

	var w io.Writer = zerolog.ConsoleWriter{NoColor: false, Out: os.Stdout, TimeFormat: time.TimeOnly}
	level := zerolog.DebugLevel

	if production {
		w = os.Stdout
		level = zerolog.InfoLevel
	}

	logger := zerolog.New(w).
		With().
		Timestamp().
		Caller().
		Logger().
		Level(level)

	return &ZerologAdapter{
		l: logger,
	}
}

// Debug implements [Logger]
func (z *ZerologAdapter) Debug(msg string, args ...any) {
	event := z.l.Debug()
	z.addFields(event, args...).Msg(msg)
}

// Info implements [Logger]
func (z *ZerologAdapter) Info(msg string, args ...any) {
	event := z.l.Info()
	z.addFields(event, args...).Msg(msg)
}

// Warn implements [Logger]
func (z *ZerologAdapter) Warn(msg string, args ...any) {
	event := z.l.Warn()
	z.addFields(event, args...).Msg(msg)
}

// Error implements [Logger]
func (z *ZerologAdapter) Error(msg string, err error, args ...any) {
	event := z.l.Error().Err(err)
	z.addFields(event, args...).Msg(msg)
}

// DebugCtx implements [Logger]
func (z *ZerologAdapter) DebugCtx(ctx context.Context, msg string, args ...any) {
	logger := z.l
	if l := GetFromContext(ctx); l != nil {
		if za, ok := l.(*ZerologAdapter); ok {
			logger = za.l
		}
	}
	event := logger.Debug()
	z.addFields(event, args...).Msg(msg)
}

// InfoCtx implements [Logger]
func (z *ZerologAdapter) InfoCtx(ctx context.Context, msg string, args ...any) {
	logger := z.l
	if l := GetFromContext(ctx); l != nil {
		if za, ok := l.(*ZerologAdapter); ok {
			logger = za.l
		}
	}
	event := logger.Info()
	z.addFields(event, args...).Msg(msg)
}

// WarnCtx implements [Logger]
func (z *ZerologAdapter) WarnCtx(ctx context.Context, msg string, args ...any) {
	logger := z.l
	if l := GetFromContext(ctx); l != nil {
		if za, ok := l.(*ZerologAdapter); ok {
			logger = za.l
		}
	}
	event := logger.Warn()
	z.addFields(event, args...).Msg(msg)
}

// ErrorCtx implements [Logger]
func (z *ZerologAdapter) ErrorCtx(ctx context.Context, msg string, err error, args ...any) {
	logger := z.l
	if l := GetFromContext(ctx); l != nil {
		if za, ok := l.(*ZerologAdapter); ok {
			logger = za.l
		}
	}
	event := logger.Error().Err(err)
	z.addFields(event, args...).Msg(msg)
}

// With implements [Logger]
func (z *ZerologAdapter) With(args ...any) Logger {
	ctx := z.l.With()

	for i := 0; i < len(args); {
		switch v := args[i].(type) {

		case group:
			ctx = ctx.Dict(v.Key, z.buildDict(v.Attrs...))
			i++

		case string:
			if i+1 < len(args) {
				ctx = z.addContextField(ctx, v, args[i+1])
				i += 2
			} else {
				i++
			}

		default:
			i++
		}
	}

	return &ZerologAdapter{l: ctx.Logger()}
}

// Helper functions

func (z *ZerologAdapter) addFields(event *zerolog.Event, args ...any) *zerolog.Event {
	// Skip:
	// 1. addFields
	// 2. wrapper (Info, InfoCtx, etc)
	event.CallerSkipFrame(2)

	for i := 0; i < len(args); {
		switch v := args[i].(type) {

		// -------- nested group --------
		case group:
			event = event.Dict(v.Key, z.buildDict(v.Attrs...))
			i++

		// -------- flat key-value --------
		case string:
			if i+1 < len(args) {
				event = z.addEventField(event, v, args[i+1])
				i += 2
			} else {
				i++ // malformed, ignore trailing key
			}

		// -------- unsupported --------
		default:
			i++ // ignore silently or panic (your choice)
		}
	}

	return event
}

func (z *ZerologAdapter) addContextField(ctx zerolog.Context, key string, value any) zerolog.Context {
	switch v := value.(type) {
	case string:
		return ctx.Str(key, v)
	case int:
		return ctx.Int(key, v)
	case int64:
		return ctx.Int64(key, v)
	case int32:
		return ctx.Int32(key, v)
	case float64:
		return ctx.Float64(key, v)
	case float32:
		return ctx.Float32(key, v)
	case bool:
		return ctx.Bool(key, v)
	case error:
		return ctx.AnErr(key, v)
	case []byte:
		return ctx.Bytes(key, v)
	default:
		return ctx.Interface(key, v)
	}
}

func (z *ZerologAdapter) addEventField(event *zerolog.Event, key string, value any) *zerolog.Event {
	switch v := value.(type) {
	case string:
		return event.Str(key, v)
	case int:
		return event.Int(key, v)
	case int64:
		return event.Int64(key, v)
	case int32:
		return event.Int32(key, v)
	case float64:
		return event.Float64(key, v)
	case float32:
		return event.Float32(key, v)
	case bool:
		return event.Bool(key, v)
	case error:
		return event.Err(v)
	case []byte:
		return event.Bytes(key, v)
	default:
		return event.Interface(key, v)
	}
}

func (z *ZerologAdapter) buildDict(attrs ...any) *zerolog.Event {
	d := zerolog.Dict()

	for i := 0; i < len(attrs); {
		switch v := attrs[i].(type) {

		case group:
			d = d.Dict(v.Key, z.buildDict(v.Attrs...))
			i++

		case string:
			if i+1 < len(attrs) {
				d = z.addEventField(d, v, attrs[i+1])
				i += 2
			} else {
				i++
			}

		default:
			i++
		}
	}

	return d
}
