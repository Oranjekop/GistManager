// Package l10n shares MyGo's locale with application text and service errors.
package l10n

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/i18n"
)

// Text uses Chinese for zh locales and English for every other language.
// An unset locale is the source language, useful for headless service callers.
func Text(source string) string { return In(i18n.Locale(), source) }

func In(locale, source string) string {
	if locale == "" || strings.HasPrefix(strings.ToLower(locale), "zh") {
		return source
	}
	if text, ok := english[source]; ok {
		return text
	}
	return source
}

// Message keeps its source and arguments until rendering, so existing notices
// follow language changes without modifying user-provided filenames or content.
type Message interface{ String() string }

type message struct {
	source string
	args   []any
	cause  error
}

func Msg(source string) Message               { return &message{source: source} }
func Msgf(source string, args ...any) Message { return &message{source: source, args: args} }
func FromError(err error) Message {
	if err == nil {
		return nil
	}
	return &message{cause: err}
}
func Render(m Message) string {
	if m == nil {
		return ""
	}
	return m.String()
}
func (m *message) String() string {
	if m.cause != nil {
		return m.cause.Error()
	}
	if m.args == nil {
		return Text(m.source)
	}
	return fmt.Sprintf(Text(m.source), m.args...)
}

func FileCount(count int) string {
	if count == 1 {
		return Text("1 个文件")
	}
	return fmt.Sprintf(Text("%d 个文件"), count)
}

// Error messages are resolved when displayed, including package-level sentinels.
func Error(source string) error { return &messageError{source: source} }

func Errorf(source string, args ...any) error {
	return &messageError{source: source, args: args}
}

type messageError struct {
	source string
	args   []any
}

func (e *messageError) Error() string {
	if e.args == nil {
		return Text(e.source)
	}
	return fmt.Errorf(Text(e.source), e.args...).Error()
}

func (e *messageError) Unwrap() error {
	for _, arg := range e.args {
		if err, ok := arg.(error); ok && strings.Contains(e.source, "%w") {
			return err
		}
	}
	return nil
}
