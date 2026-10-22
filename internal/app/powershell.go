package app

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
)

var powerShell = &editor.Language{
	Name: "PowerShell", Extensions: []string{".ps1", ".psm1"}, LineComment: "#", BlockComment: [2]string{"<#", "#>"}, TabSize: 2, WordChars: "-",
	Brackets:  []editor.BracketPair{{Open: "{", Close: "}"}, {Open: "(", Close: ")"}, {Open: "[", Close: "]"}},
	AutoClose: []editor.BracketPair{{Open: "{", Close: "}"}, {Open: "(", Close: ")"}, {Open: "[", Close: "]"}, {Open: "\"", Close: "\""}, {Open: "'", Close: "'"}},
	Tokenizer: editor.TokenizerFunc(tokenizePowerShell),
}

func tokenizePowerShell(line string, state uint64, tokens []editor.Token) ([]editor.Token, uint64) {
	emit := func(end int, kind editor.TokenKind) { tokens = append(tokens, editor.Token{End: end, Kind: kind}) }
	for i := 0; i < len(line); {
		if state == 1 || (state == 0 && strings.HasPrefix(line[i:], "<#")) {
			end := strings.Index(line[i:], "#>")
			if end < 0 {
				emit(len(line), editor.TokenComment)
				return tokens, 1
			}
			i += end + 2
			emit(i, editor.TokenComment)
			state = 0
			continue
		}
		if state == 4 || state == 5 {
			end := "'@"
			if state == 5 {
				end = "\"@"
			}
			emit(len(line), editor.TokenString)
			if strings.HasPrefix(strings.TrimSpace(line), end) {
				state = 0
			}
			return tokens, state
		}
		if state == 0 && (strings.HasPrefix(line[i:], "@'") || strings.HasPrefix(line[i:], "@\"")) {
			state = 4
			if line[i+1] == '"' {
				state = 5
			}
			emit(len(line), editor.TokenString)
			return tokens, state
		}
		if state == 2 || state == 3 || line[i] == '\'' || line[i] == '"' {
			quote := byte('\'')
			if state == 3 || (state == 0 && line[i] == '"') {
				quote = '"'
			}
			if state == 0 {
				i++
			}
			state = 2
			if quote == '"' {
				state = 3
			}
			for i < len(line) {
				if quote == '"' && line[i] == '`' && i+1 < len(line) {
					i += 2
					continue
				}
				if line[i] == quote {
					i++
					if i < len(line) && line[i] == quote {
						i++
						continue
					}
					state = 0
					break
				}
				i++
			}
			emit(i, editor.TokenString)
			continue
		}
		if line[i] == '#' {
			emit(len(line), editor.TokenComment)
			return tokens, 0
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		kind := editor.TokenText
		if r == '$' || unicode.IsLetter(r) || r == '_' || unicode.IsDigit(r) {
			start := i
			i += size
			for i < len(line) {
				next, width := utf8.DecodeRuneInString(line[i:])
				if !unicode.IsLetter(next) && !unicode.IsDigit(next) && !strings.ContainsRune("_-:", next) {
					break
				}
				i += width
			}
			word := strings.ToLower(line[start:i])
			if r == '$' {
				kind = editor.TokenVariable
			} else if unicode.IsDigit(r) {
				kind = editor.TokenNumber
			} else if strings.Contains("|function|filter|param|begin|process|end|if|elseif|else|switch|for|foreach|while|do|until|break|continue|return|try|catch|finally|throw|class|using|in|", "|"+word+"|") {
				kind = editor.TokenKeyword
			} else if strings.ContainsRune(word, '-') {
				kind = editor.TokenFunction
			}
		} else {
			i += size
		}
		emit(i, kind)
	}
	return tokens, state
}
