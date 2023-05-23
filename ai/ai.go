package ai

import (
	"log"
	"os"
	"strings"
	"unicode"

	"github.com/jdkato/prose/v2"
)

type TokenizedSentence struct {
	Sentence   string
	TokenCount int
}

func TokenizeSentence(text string) (sentences []TokenizedSentence, err error) {
	doc, err := prose.NewDocument(text)
	if err != nil {
		logger.Println("ERROR:", err.Error())
		return
	}

	for _, sent := range doc.Sentences() {
		sentenceText := sent.Text
		_, tokenCount := Tokenize(sentenceText)

		sentences = append(sentences, TokenizedSentence{
			Sentence:   sentenceText,
			TokenCount: tokenCount,
		})
	}

	return
}

func Tokenize(text string) (tokens []string, tokenCount int) {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(".,:;'\"?!()-_", r)
	})

	// for _, field := range fields {
	// 	tokens = append(tokens, field)
	// }
	tokens = append(tokens, fields...)

	// Add punctuation as separate tokens
	for _, char := range text {
		if strings.ContainsRune(".,:;'\"?!()-_", char) {
			tokens = append(tokens, string(char))
		}
	}

	tokenCount = len(tokens)

	return
}

func EsitmateCost(tokenCount int, costPer1000Tokens float64) (cost float64) {
	cost = float64(tokenCount) * costPer1000Tokens / 1000

	return
}

var (
	logger *log.Logger
)

func init() {
	logger = log.New(os.Stdout, "[AI] ", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)
}
