package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"riv247/jtg/model"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/k0kubun/pp"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/text/unicode/norm"
)

func handlePromptRequest(c echo.Context) (err error) {
	return c.JSON(http.StatusOK, masterPromptStr)
}

type promptParams map[string]interface{}

type promptCommonStruct struct {
	Provider      string `json:"provider,omitempty"`
	ProviderID    string `json:"provider_id,omitempty"`
	PromptVersion string `json:"v,omitempty"`
}

type promptReqStruct struct {
	promptCommonStruct                // `json:"prompt_common_struct,omitempty"`
	Params               promptParams `json:"p,omitempty"`
	OptionalInstructions string       `json:"oi,omitempty"`
	Text                 string       `json:"t,omitempty"`
}

func (promptReq promptReqStruct) JSON() (b []byte, err error) {
	b, err = json.Marshal(promptReq)
	return
}

type promptResStruct struct {
	promptCommonStruct        // `json:"prompt_common_struct,omitempty"`
	Context            string `json:"c,omitempty"`
	Summary            string `json:"s,omitempty"`
	TLDR               string `json:"tldr,omitempty"`
}

func handleTextRequest(c echo.Context) (err error) {
	reqPrompt := new(promptReqStruct)
	if err = c.Bind(reqPrompt); err != nil {
		return
	}

	// TODO: reqPrompt.Validate()

	prompt, err := reqPrompt.JSON()
	if err != nil {
		return
	}
	promptStr := string(prompt)

	ctx := context.Background()

	// TODO: jtg/ai package
	// client := ai.NewClient(os.Getenv("OPEN_AI_API_KEY"))

	client := openai.NewClient(os.Getenv("OPEN_AI_API_KEY"))
	res, err := client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: promptStr,
				},
			},
		},
	)
	if err != nil {
		logger.Printf("ChatCompletion error: %v\n", err)
		return
	}

	pp.Println(res)
	pp.Println(res.Choices[0].Message.Content)

	return c.JSON(http.StatusOK, res.Choices[0].Message.Content)
}

func corsForHandleTestRequest(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Set CORS headers for the response
		c.Response().Header().Set("Access-Control-Allow-Origin", "*")
		c.Response().Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		c.Response().Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if c.Request().Method == "OPTIONS" {
			return c.NoContent(http.StatusNoContent)
		}

		return next(c)
	}
}

const (
	// Model cost per 1,000 tokens
	gpt35TurboCostPer1000Tokens = 0.002
)

func estimateCost(totalTokens int, costPer1000Tokens float64) (cost float64) {
	cost = float64(totalTokens) * costPer1000Tokens / 1000

	return
}

func countTokens(s string) int {
	var tokenCount int
	iter := norm.Iter{}
	iter.InitString(norm.NFKC, s)

	for !iter.Done() {
		next := iter.Next()
		r, _ := utf8.DecodeRune(next)
		if !unicode.IsSpace(r) && !strings.ContainsRune(".,:;'\"?!()-_", r) {
			tokenCount++
		}
	}
	return tokenCount
}

func handleTestRequest(c echo.Context) (err error) {
	reqPrompt := new(promptReqStruct)
	if err = c.Bind(reqPrompt); err != nil {
		return
	}

	// lines := strings.Split(reqPrompt.Text, "\n")
	// reqPrompt.Text = fmt.Sprintf("[%s]", strings.Join(lines, ", "))

	b, err := json.Marshal(reqPrompt)
	if err != nil {
		logger.Printf("Marshal error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Set the response header
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextPlainCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)

	// Create an OpenAI client
	aiClient := openai.NewClient(os.Getenv("OPEN_AI_API_KEY"))
	ctx := context.Background()

	req := openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo,
		// MaxTokens: 20,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: masterPromptStr + " " + string(b),
			},
		},
		Stream: true,
	}
	stream, err := aiClient.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return err
	}
	defer stream.Close()

	pp.Println(req.Messages[0].Content)

	var content string
	for {
		res, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		// Write the partial response to the client
		partialContent := res.Choices[0].Delta.Content
		if _, err := c.Response().Write([]byte(partialContent)); err != nil {
			return err
		}
		content = content + partialContent

		// Flush the response writer
		if flusher, ok := c.Response().Writer.(http.Flusher); ok {
			flusher.Flush()
		}
	}

	// Count tokens in the partial response
	tokenCounter := countTokens(content)

	// Calculate the estimated cost
	gpt35TurboCost := estimateCost(tokenCounter, gpt35TurboCostPer1000Tokens)

	// Print the estimated costs
	pp.Println(fmt.Sprintf("Estimated cost for GPT-3.5-turbo: $%.4f", gpt35TurboCost))

	pp.Println(content)

	// check content is JSON
	// if not, return error
	err = json.Unmarshal([]byte(content), &promptResStruct{})
	if err != nil {
		logger.Printf("Unmarshal error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	var summaryModel model.SummaryModel
	err = json.Unmarshal([]byte(content), &summaryModel)
	if err != nil {
		logger.Printf("Unmarshal error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	summaryModel.ProviderID = reqPrompt.ProviderID
	summaryModel.Provider = reqPrompt.Provider
	summaryModel.PromptVersion = reqPrompt.PromptVersion
	summaryModel.CreatedAt = time.Now()
	summaryModel.UpdatedAt = time.Now()

	pp.Println(summaryModel)
	// return

	err = summaryModel.Save(dbClient)
	if err != nil {
		logger.Printf("Save error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	return
}

var (
	masterPromptStr string

	dbClient *dynamodb.Client
)

func main() {
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Configure CORS middleware for the /test endpoint
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{echo.POST, echo.OPTIONS},
		AllowHeaders:     []string{echo.HeaderContentType},
		ExposeHeaders:    []string{},
		AllowCredentials: false,
	}))

	// Routes
	e.GET("/prompt", handlePromptRequest)
	e.POST("/test", handleTestRequest, corsForHandleTestRequest)
	e.POST("/text", handleTextRequest)

	// Start server
	e.Logger.Fatal(e.Start(fmt.Sprintf(":%s", os.Getenv("JTG_SERVER_PORT"))))
}

var (
	logger *log.Logger
)

func init() {
	logger = log.New(os.Stdout, "[JTG] ", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)

	promptBytes, err := os.ReadFile("./prompt/prompt_small_0-0-1.txt")
	if err != nil {
		logger.Printf("ReadFile error: %v\n", err)
		return
	}

	fields := strings.Fields(string(promptBytes))
	masterPromptStr = strings.Join(fields, " ")
	masterPromptStr = strings.ReplaceAll(masterPromptStr, "--", "")

	dbClient, err = model.NewClient()
	if err != nil {
		logger.Printf("NewClient error: %v\n", err)
		return
	}

	model.MakeTables(dbClient)
}
