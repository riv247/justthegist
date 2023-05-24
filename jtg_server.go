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
	"riv247/jtg/ai"
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

	"riv247/jtg/provider"
)

func handlePromptRequest(c echo.Context) (err error) {
	return c.JSON(http.StatusOK, ai.SummarizePrompt)
}

type promptTokenizeReqStruct struct {
	Input  string `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
}

func handlePromptTokenizeRequest(c echo.Context) (err error) {
	var req promptTokenizeReqStruct
	if err = c.Bind(&req); err != nil {
		logger.Println("ERROR:", err.Error())
		return
	}

	if req.Input == "" {
		err = errors.New("input is required")
		logger.Println("ERROR:", err.Error())

		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": ".input is required",
		})
	}

	aiClient := ai.NewClient(os.Getenv("OPEN_AI_API_KEY"), openai.GPT3Dot5Turbo)
	// aiClient.DryRun = true

	promptInput := ai.PromptReqStruct{
		Text: req.Input,
	}

	b, err := json.Marshal(promptInput)
	if err != nil {
		logger.Println("ERROR:", err.Error())

		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}
	formattedInput := string(b)

	output, err := aiClient.Prompt(ai.SummarizePrompt, formattedInput)
	if err != nil {
		logger.Println("ERROR:", err.Error())

		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	var promptRes ai.PromptResStruct
	err = json.Unmarshal([]byte(output), &promptRes)
	if err != nil {
		logger.Println("ERROR:", err.Error())

		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}
	pp.Println(promptRes)

	return c.JSON(http.StatusOK, map[string]string{
		"output": output,
	})
}

func handleTextRequest(c echo.Context) (err error) {
	reqPrompt := new(ai.PromptReqStruct)
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
	reqPrompt := new(ai.PromptReqStruct)
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
				Content: ai.SummarizePrompt + " " + string(b),
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
	err = json.Unmarshal([]byte(content), &ai.PromptResStruct{})
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
	dbClient *dynamodb.Client
)

func main() {
	e := echo.New()

	// e.Use(customLogger())
	// e.Use(middleware.Logger())
	// e.Use(middleware.Recover())

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{echo.POST, echo.OPTIONS},
		AllowHeaders:     []string{echo.HeaderContentType},
		ExposeHeaders:    []string{},
		AllowCredentials: false,
	}))

	// Routes
	e.GET("/prompt", handlePromptRequest)
	e.POST("/prompt/tokenize", handlePromptTokenizeRequest)
	// e.POST("/test", handleTestRequest, corsForHandleTestRequest)
	// e.POST("/text", handleTextRequest)

	e.POST("/slack/shortcut", provider.HandleSlackInteractionRequest)

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
		logger.Fatalln("ERROR:", err.Error())
	}

	fields := strings.Fields(string(promptBytes))
	ai.SummarizePrompt = strings.Join(fields, " ")
	ai.SummarizePrompt = strings.ReplaceAll(ai.SummarizePrompt, "--", "")

	dbClient, err := model.NewClient()
	if err != nil {
		logger.Fatalln("ERROR:", err.Error())
	}

	model.MakeTables(dbClient)
}
