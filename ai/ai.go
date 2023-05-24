package ai

import (
	"log"
	"os"
	"strings"
	"unicode"

	"github.com/jdkato/prose/v2"
)

type TokenizedChunk struct {
	Sentences  []string
	TokenCount int
}

func TokenizeChunks(text string, maxTokenCount int) (chunks []TokenizedChunk, tokenCount int, err error) {
	sentences, err := TokenizeSentence(text)
	if err != nil {
		logger.Println("ERROR:", err.Error())
		return
	}
	// pp.Println("sentences:", sentences)

	// chunks = []TokenizedChunk{}
	chunk := TokenizedChunk{}
	for i, sentence := range sentences {
		nextChunkTokenCount := chunk.TokenCount + sentence.TokenCount

		// adding sentence to chunk will not fit maxTokenCount
		if nextChunkTokenCount >= maxTokenCount {
			chunks = append(chunks, chunk)
			// pp.Println("chunk:", chunk)

			// reset for next chunk
			chunk = TokenizedChunk{
				Sentences:  []string{},
				TokenCount: 0,
			}
			nextChunkTokenCount = 0 + sentence.TokenCount
		}

		// sentence is too long to fit in any chunk
		if chunk.TokenCount == 0 && nextChunkTokenCount > maxTokenCount {
			logger.Println("ERROR: sentence too long:", sentence)
			continue
		}

		// adding sentence to chunk will fit maxTokenCount
		if nextChunkTokenCount <= maxTokenCount {
			chunk.Sentences = append(chunk.Sentences, sentence.Sentence)
			chunk.TokenCount += sentence.TokenCount
			// pp.Println("chunk:", chunk)
		}

		lastSentence := i == len(sentences)-1
		if lastSentence {
			chunks = append(chunks, chunk)
			// pp.Println("chunk:", chunk)
		}
	}
	// pp.Println("chunks:", chunks)
	// pp.Println("chunksTotalTokenCount:", chunksTotalTokenCount, inputTokenCount)

	for _, chunk := range chunks {
		tokenCount += chunk.TokenCount
	}

	return
}

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
