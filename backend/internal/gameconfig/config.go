package gameconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

type Document struct {
	TemplateID      string `json:"templateId"`
	TemplateVersion string `json:"templateVersion"`
	ConfigVersion   int    `json:"configVersion"`
	Config          Config `json:"config"`
}

type Config struct {
	OpeningTitle   string           `json:"openingTitle"`
	Rounds         []map[string]any `json:"rounds"`
	LoveLetter     string           `json:"loveLetter,omitempty"`
	LetterPassword string           `json:"letterPassword,omitempty"`
	PasswordHint   string           `json:"passwordHint,omitempty"`
}

func (document Document) Validate() error {
	if document.ConfigVersion != 1 {
		return errors.New("unsupported game config version")
	}
	switch document.TemplateID {
	case "memory-game":
		if document.TemplateVersion != "1.0.0" {
			return errors.New("unsupported game config version")
		}
	case "love-journey":
		if document.TemplateVersion != "1.0.0" && document.TemplateVersion != "1.1.0" {
			return errors.New("unsupported game config version")
		}
	default:
		return errors.New("unsupported game config version")
	}
	titleLength := utf8.RuneCountInString(document.Config.OpeningTitle)
	if titleLength < 1 || titleLength > 120 || len(document.Config.Rounds) > 100 {
		return errors.New("invalid game config content")
	}
	if document.TemplateID == "love-journey" && document.TemplateVersion == "1.1.0" {
		letterLength := utf8.RuneCountInString(strings.TrimSpace(document.Config.LoveLetter))
		hintLength := utf8.RuneCountInString(strings.TrimSpace(document.Config.PasswordHint))
		if letterLength < 1 || letterLength > 1000 || hintLength > 100 || !isFourDigitLetterPassword(document.Config.LetterPassword) {
			return errors.New("invalid love journey material config")
		}
	}
	return nil
}

func isFourDigitLetterPassword(password string) bool {
	if len(password) != 4 {
		return false
	}
	for _, digit := range password {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func Decode(data []byte) (Document, error) {
	var document Document
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return Document{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Document{}, errors.New("game config must contain one JSON document")
	}
	if err := document.Validate(); err != nil {
		return Document{}, err
	}
	return document, nil
}
