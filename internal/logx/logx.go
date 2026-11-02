// Package logx is a small thread-safe progress logger with stable prefixes.
// The visual language matches Subsoil: [*] progress, [+] result, [!] warning,
// [v] detail.
package logx

import (
	"fmt"
	"io"
	"sync"
)

// Activity is an optional live display that is paused while a line is written.
type Activity interface {
	Pause()
	Resume()
}

// Logger writes prefixed progress lines. A nil *Logger is safe to use.
type Logger struct {
	mu       sync.Mutex
	w        io.Writer
	verbose  bool
	activity Activity
}

// New returns a logger writing to w. Verbose lines are suppressed unless
// verbose is true.
func New(w io.Writer, verbose bool) *Logger {
	return &Logger{w: w, verbose: verbose}
}

// SetActivity installs or clears a live display.
func (l *Logger) SetActivity(a Activity) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.activity = a
	l.mu.Unlock()
}

func (l *Logger) write(prefix, format string, args ...any) {
	if l == nil || l.w == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.activity != nil {
		l.activity.Pause()
	}
	fmt.Fprintf(l.w, prefix+format+"\n", args...)
	if l.activity != nil {
		l.activity.Resume()
	}
}

// Info is normal progress output.
func (l *Logger) Info(format string, args ...any) { l.write("[*] ", format, args...) }

// Success marks a useful result.
func (l *Logger) Success(format string, args ...any) { l.write("[+] ", format, args...) }

// Warn marks a recoverable problem.
func (l *Logger) Warn(format string, args ...any) { l.write("[!] ", format, args...) }

// Error marks a failure that does not stop the whole run.
func (l *Logger) Error(format string, args ...any) { l.write("[x] ", format, args...) }

// Verbose is per-item detail, shown only with --verbose.
func (l *Logger) Verbose(format string, args ...any) {
	if l == nil || !l.verbose {
		return
	}
	l.write("[v] ", format, args...)
}
