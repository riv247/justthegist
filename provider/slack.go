package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"riv247/jtg/ai"
	"strings"
	"time"

	"github.com/k0kubun/pp"
	"github.com/labstack/echo/v4"
	"github.com/sashabaranov/go-openai"
	"github.com/slack-go/slack"
)

// demo users for building messages to test summarization
var userTokens = map[string]string{
	"user1": os.Getenv("USER_1_SLACK_TOKEN"),
	"user2": os.Getenv("USER_2_SLACK_TOKEN"),
	"user3": os.Getenv("USER_3_SLACK_TOKEN"),
	"user4": os.Getenv("USER_4_SLACK_TOKEN"),
	"user5": os.Getenv("USER_5_SLACK_TOKEN"),
}

type mockMessagesModalView struct {
	Header string `json:"header"`
	Body   string `json:"body"`
}

func (m mockMessagesModalView) View() (view slack.ModalViewRequest, err error) {
	s := strings.TrimSpace(`--json
{
	"type": "modal",
	"callback_id": "mock_messages",

	"title": {
		"type": "plain_text",
		"text": "Generate Mock Messages?",
		"emoji": true
	},

	"submit": {
		"type": "plain_text",
		"text": "Yes",
		"emoji": true
	},

	"close": {
		"type": "plain_text",
		"text": "No",
		"emoji": true
	},

	"blocks": [
		{
			"block_id": "block_conversation",
			"type": "input",
			"label": {
				"type": "plain_text",
				"text": "Select a channel"
			},
			"element": {
				"type": "conversations_select",
				"action_id": "mock_messages_conversation",
				"placeholder": {
					"type": "plain_text",
					"text": "Select a channel"
				}
			}
		},

		{
			"block_id": "block_topic",
			"type": "input",
			"optional": true,
			"element": {
				"type": "plain_text_input",
				"multiline": true,
				"action_id": "mock_messages_topic"
			},
			"label": {
				"type": "plain_text",
				"text": "Topic",
				"emoji": true
			}
		}
	]
}
`)

	s = strings.Join(strings.Fields(s), " ")
	s = strings.Replace(s, "--json", "", 1)

	err = json.Unmarshal([]byte(s), &view)
	if err != nil {
		logger.Printf("Error unmarshalling confirm view: %s", err)
		return
	}

	return
}

func handleMockMessagesViewSubmission(payload *slack.InteractionCallback) (err error) {
	// userIDs := payload.View.State.Values["block_users"]["mock_messages_users"].SelectedUsers
	channelID := payload.View.State.Values["block_conversation"]["mock_messages_conversation"].SelectedConversation
	topic := payload.View.State.Values["block_topic"]["mock_messages_topic"].Value
	pp.Println(channelID, topic)

	// get a list of users in the channel
	api := slack.New(os.Getenv("SLACK_BOT_TOKEN"))
	userIDs, _, err := api.GetUsersInConversation(&slack.GetUsersInConversationParameters{
		ChannelID: channelID,
	})
	if err != nil {
		logger.Printf("Error getting users in conversation: %s", err)
	}

	usersMap := map[string]string{}
	for _, userID := range userIDs {
		// get user info
		user, err := api.GetUserInfo(userID)
		if err != nil {
			logger.Printf("Error getting user info: %s", err)
		}

		usersMap[user.RealName] = user.ID
	}

	// ALWAYS include 1-2 message(s) 4-5 sentences in length.
	// ALWAYS include 1-2 message(s) mentioning the previous message user(s) as a response.
	// NEVER add newlines to the JSON array, keep in minified.
	// NEVER respond with more than the single expected JSON array.

	// Each messages must be 4-5 sentences in length. If it doesn't have 4-5 punctuation marks, it is too short.

	// {
	//   "prompt": "  RULES:  OUTPUT: {"messages": [["user1","I had an interesting discussion about AI and machine learning with user2 and user3. They both brought some insightful perspectives to the table."], ["user2","Indeed, user1. I found your thoughts on the ethics of AI particularly thought-provoking."]]}"
	// }

	promptStr := strings.TrimSpace(`
You are a sophisticated AI text generator.

INSTRUCTIONS:
- Construct a conversation with 5-10 messages about any topic you choose.
- Each message should be detailed and elaborate, comparable to a short paragraph, not just a single sentence. Make sure to include a variety of information and interesting points in each message.
- You can use any combination of users 1-5.
- ALWAYS mention user(s) in some of your responses.

RULES:
- Always maintain the correct format, which is a single JSON object that encapsulates the entire conversation.

OUTPUT: 
{
	"messages":	[
		["user1","user2 and user3 are cool"],
		["user2","good point user1"]
	]
}
`)
	if topic != "" {
		promptStr = strings.Replace(promptStr, "any topic you choose", topic, 1)
	}

	ctx := context.TODO()

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
	pp.Println(res.Choices[0].Message.Content)
	fmt.Println(res.Choices[0].Message.Content)

	content := strings.TrimSpace(res.Choices[0].Message.Content)
	content = strings.Join(strings.Fields(content), " ")
	fmt.Println(content)

	type aiResStruct struct {
		Messages [][]string `json:"messages"`
	}

	var aiRes aiResStruct
	err = json.Unmarshal([]byte(content), &aiRes)
	if err != nil {
		logger.Printf("Error unmarshalling messages: %s", err)
		return
	}

	for _, userMessage := range aiRes.Messages {
		userName := userMessage[0]
		message := userMessage[1]

		if userToken, exists := userTokens[userName]; exists {
			for realName, userID := range usersMap {
				message = strings.ReplaceAll(message, realName, "<@"+userID+">")

				RealName := strings.ToUpper(realName[0:1]) + realName[1:]
				message = strings.ReplaceAll(message, RealName, "<@"+userID+">")
			}

			userAPI := slack.New(userToken)
			_, _, err = userAPI.PostMessage(
				channelID,
				slack.MsgOptionText(message, false),
			)
			if err != nil {
				logger.Printf("Error posting message: %s", err)
				return
			}

			// Create a new random source with a fixed seed
			source := rand.NewSource(time.Now().UnixNano())
			// Create a new random generator using the source
			generator := rand.New(source)
			// Sleep a random number between 1 and 3 seconds
			time.Sleep(time.Duration(generator.Intn(3-1)+1) * time.Second)
		}
	}

	return
}

type summaryModalView struct {
	Header  string `json:"header"`
	Summary string `json:"summary"`
	TLDR    string `json:"tldr"`
}

func (summary summaryModalView) View() (view slack.ModalViewRequest, err error) {
	s := strings.TrimSpace(`--json
{
	"type": "modal",
	"title": {
		"type": "plain_text",
		"text": "{{HEADER}}",
		"emoji": true
	},
	"blocks": [
		{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "*Summary*:"
			}
		},
		{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "{{SUMMARY}}"
			}
		},
		{
			"type": "divider"
		},
		{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "*TLDR;*"
			}
		},
		{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "{{TLDR}}"
			}
		},
		{
			"type": "divider"
		},
		{
			"type": "context",
			"elements": [
				{
					"type": "plain_text",
					"text": "This summary was generated by an AI text generator. It may not be accurate.",
					"emoji": true
				}
			]
		},
		{
			"type": "actions",
			"elements": [
				{
					"type": "button",
					"text": {
						"type": "plain_text",
						"text": "Share TLDR;",
						"emoji": true
					},
					"value": "tldr"
				},
				{
					"type": "button",
					"text": {
						"type": "plain_text",
						"text": "Share Summary",
						"emoji": true
					},
					"value": "share",
					"url": "https://google.com"
				}
			]
		}
	]
}
`)

	s = strings.Join(strings.Fields(s), " ")
	s = strings.Replace(s, "--json", "", 1)

	s = strings.Replace(s, "{{HEADER}}", summary.Header, 1)
	s = strings.Replace(s, "{{SUMMARY}}", summary.Summary, 1)
	s = strings.Replace(s, "{{TLDR}}", summary.TLDR, 1)
	s = strings.ReplaceAll(s, "\n", "\\n")

	logger.Println(s)

	err = json.Unmarshal([]byte(s), &view)
	if err != nil {
		logger.Printf("Error unmarshalling summary view: %s", err)
		return
	}

	return
}

func init() {
	os.Setenv("SLACK_BOT_TOKEN", "xoxb-5181488852848-5258684481921-i7xnuD8Sa4RqkDu7CafezvpI")
	os.Setenv("SLACK_SIGNING_SECRET", "70495d24af8e403763f285de6200aabe")
}

func verifyRequest(r *http.Request) (err error) {
	// Verify the request by checking the signing secret
	verifier, err := slack.NewSecretsVerifier(r.Header, os.Getenv("SLACK_SIGNING_SECRET"))
	if err != nil {
		logger.Printf("Error creating verifier: %s", err)
		return
	}

	bodyReader := io.TeeReader(r.Body, &verifier)
	if _, err = io.Copy(io.Discard, bodyReader); err != nil {
		logger.Printf("Error reading request body: %s", err)
		return
	}

	if err = verifier.Ensure(); err != nil {
		logger.Printf("Error verifying request: %s", err)
		return
	}

	return
}

func summarizeMessage(payload *slack.InteractionCallback) (summaryModal summaryModalView, err error) {
	promptInput := ai.PromptReqStruct{
		Text: payload.Message.Msg.Text,
	}

	b, err := json.Marshal(promptInput)
	if err != nil {
		logger.Println("ERROR:", err.Error())

		return
	}
	formattedInput := string(b)

	aiClient := ai.NewClient(os.Getenv("OPEN_AI_API_KEY"), openai.GPT3Dot5Turbo)
	output, err := aiClient.Prompt(ai.SummarizePrompt, formattedInput)
	if err != nil {
		logger.Println("ERROR:", err.Error())

		return
	}

	var promptRes ai.PromptResStruct
	err = json.Unmarshal([]byte(output), &promptRes)
	if err != nil {
		logger.Println("ERROR:", err.Error())

		return
	}
	pp.Println(promptRes)

	summaryList := ""
	for _, line := range promptRes.Summary {
		summaryList += fmt.Sprintf("\u2022 %s\n", line)
	}

	summaryModal = summaryModalView{
		Header:  "Header",
		Summary: summaryList,
		TLDR:    promptRes.TLDR,
	}

	return
}

func summarizeMessages(payload *slack.InteractionCallback) (summaryModal summaryModalView, err error) {
	promptInput := ai.PromptReqStruct{
		Text: payload.Message.Msg.Text,
	}

	api := slack.New(os.Getenv("SLACK_BOT_TOKEN"))
	history, err := api.GetConversationHistory(&slack.GetConversationHistoryParameters{
		ChannelID: payload.Channel.ID,
		Oldest:    payload.Message.Msg.Timestamp,
		// Inclusive: true,
		Limit: 20,
	})
	if err != nil {
		logger.Println("ERROR:", err.Error())

		return
	}

	messages := []string{}
	if len(history.Messages) > 0 {
		for _, msg := range history.Messages {
			// if msg.BotID != "" {
			// 	continue
			// }

			// ignore any messages about being invited to channel
			// ignore any messages about joining/leaving the channel
			if msg.SubType == "channel_invite" || msg.SubType == "channel_leave" || msg.SubType == "channel_join" {
				continue
			}

			// add to start of messages
			messages = append([]string{msg.Text}, messages...)
		}
		messages = append([]string{promptInput.Text}, messages...)

		// pp.Println(messages)
	}

	promptInput.Text = strings.Join(messages, "\n")
	fmt.Println(promptInput.Text)
	// return summaryModal, fmt.Errorf("Not implemented")

	b, err := json.Marshal(promptInput)
	if err != nil {
		logger.Println("ERROR:", err.Error())

		return
	}
	formattedInput := string(b)

	aiClient := ai.NewClient(os.Getenv("OPEN_AI_API_KEY"), openai.GPT3Dot5Turbo)
	output, err := aiClient.Prompt(ai.SummarizePrompt, formattedInput)
	if err != nil {
		logger.Println("ERROR:", err.Error())

		return
	}

	var promptRes ai.PromptResStruct
	err = json.Unmarshal([]byte(output), &promptRes)
	if err != nil {
		logger.Println("ERROR:", err.Error())

		return
	}
	pp.Println(promptRes)

	summaryList := ""
	for _, line := range promptRes.Summary {
		summaryList += fmt.Sprintf("\u2022 %s\n", line)
	}

	summaryModal = summaryModalView{
		Header:  "Header",
		Summary: summaryList,
		TLDR:    promptRes.TLDR,
	}

	return
}

func HandleSlackInteractionRequest(c echo.Context) (err error) {
	// if err = verifyRequest(c.Request()); err != nil {
	// 	return c.NoContent(http.StatusUnauthorized)
	// }

	payload := new(slack.InteractionCallback)
	if err = json.Unmarshal([]byte(c.FormValue("payload")), payload); err != nil {
		logger.Printf("Error unmarshalling payload: %s", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	pp.Println(payload.Type)
	// pp.Println(payload.Type, payload)

	// Perform actions based on the interaction callback
	switch payload.Type {
	case slack.InteractionTypeShortcut:
		return handleGlobalShortcut(payload)
	case slack.InteractionTypeViewSubmission:
		pp.Println(payload, payload.View.CallbackID)

		if payload.View.CallbackID == "mock_messages" {
			return handleMockMessagesViewSubmission(payload)
		}

		// api := slack.New(os.Getenv("SLACK_BOT_TOKEN"))
		// _, err = api.PostEphemeral("", payload.User.ID, slack.MsgOptionText("This shortcut can only be used in a channel", false), slack.MsgOptionPostEphemeral(payload.TriggerID))
		// if err != nil {
		// 	logger.Printf("Error posting ephemeral message: %s", err)
		// 	return
		// }

		// for key, val := range payload.View.State.Values {
		// 	pp.Println(key, val)
		// }

		// Handle modal view submission
		// TODO: Implement your logic here
	case slack.InteractionTypeBlockActions:
		// Handle block actions (e.g., button clicks)
		// TODO: Implement your logic here
	case slack.InteractionTypeMessageAction:
		pp.Println(payload.CallbackID)

		summaryModal := summaryModalView{
			Header:  "Summarizing...",
			Summary: "Your text is being summarized. Please wait...",
			TLDR:    "N/A",
		}

		view, err := summaryModal.View()
		if err != nil {
			logger.Println("ERROR:", err.Error())

			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"error": err.Error(),
			})
		}

		api := slack.New(os.Getenv("SLACK_BOT_TOKEN"))
		openViewRes, err := api.OpenView(payload.TriggerID, view)
		if err != nil {
			logger.Println("ERROR:", err.Error())

			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"error": err.Error(),
			})
		}

		// TODO: move to handler method

		if payload.CallbackID == "jtg_this_message" {
			summaryModal, err = summarizeMessage(payload)
		}
		if payload.CallbackID == "jtg_from_message" {
			summaryModal, err = summarizeMessages(payload)
		}

		if err != nil {
			logger.Println("ERROR:", err.Error())

			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"error": err.Error(),
			})
		}

		view, err = summaryModal.View()
		if err != nil {
			logger.Println("ERROR:", err.Error())

			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"error": err.Error(),
			})
		}

		externalID := openViewRes.View.ExternalID
		hash := openViewRes.View.Hash
		viewID := openViewRes.View.ID

		_, err = api.UpdateView(view, externalID, hash, viewID)
		if err != nil {
			logger.Println("ERROR:", err.Error())

			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"error": err.Error(),
			})
		}

		return c.NoContent(http.StatusOK)

		return handleMessageShortcut(payload)

	case slack.InteractionTypeInteractionMessage:
		// Handle message actions (e.g., message button clicks)
		// TODO: Implement your logic here

		// Get the button action value
		actionValue := payload.ActionCallback.AttachmentActions[0].Value
		pp.Println(actionValue)

		// Perform actions based on the button action value
		switch actionValue {
		case "button1_value":
			// Action for Button 1
			// TODO: Implement your logic here
		case "button2_value":
			// Action for Button 2
			// TODO: Implement your logic here
		default:
			logger.Printf("Unknown button action value: %s", actionValue)

			// Unknown button action value
			return c.String(http.StatusBadRequest, "Unknown button action value")
		}
	default:
		logger.Printf("Unexpected payload type: %s", payload.Type)

		// Unknown interaction type
		return c.String(http.StatusBadRequest, "Unknown interaction type")
	}

	// // Return a JSON response
	// return c.JSON(http.StatusOK, map[string]string{
	// 	"response_type": "ephemeral",
	// 	"text":          "Interaction handled!",
	// })

	// switch payload.Type {
	// case slack.InteractionTypeShortcut:
	// 	if err = handleMessageShortcut(*payload); err != nil {
	// 		return c.NoContent(http.StatusInternalServerError)
	// 	}
	// case slack.InteractionTypeViewSubmission:
	// 	if err = handleViewSubmission(*payload); err != nil {
	// 		return c.NoContent(http.StatusInternalServerError)
	// 	}
	// default:
	// 	logger.Printf("Unexpected payload type: %s", payload.Type)
	// 	return c.NoContent(http.StatusInternalServerError)
	// }

	return c.NoContent(http.StatusOK)
}

func handleGlobalShortcut(payload *slack.InteractionCallback) (err error) {
	api := slack.New(os.Getenv("SLACK_BOT_TOKEN"))

	if payload.Channel.ID == "" {
		mockMessagesModal := mockMessagesModalView{}

		var view slack.ModalViewRequest
		view, err = mockMessagesModal.View()
		if err != nil {
			return
		}

		_, err = api.OpenView(payload.TriggerID, view)
		if err != nil {
			logger.Printf("Error opening view: %s", err)
			return
		}

		return
	}

	// get a list of users in the channel
	members, _, err := api.GetUsersInConversation(&slack.GetUsersInConversationParameters{
		ChannelID: payload.Channel.ID,
	})
	if err != nil {
		logger.Printf("Error getting users in conversation: %s", err)

		// Send a response message
		api.PostEphemeral(payload.Channel.ID, payload.User.ID, slack.MsgOptionText("Error getting users in conversation", false))
	}

	for _, member := range members {
		// Post a message as a user
		params := slack.PostMessageParameters{
			Username: member,
			AsUser:   true,
		}

		// post a message as the member
		_, _, err = api.PostMessage(payload.Channel.ID, slack.MsgOptionText("ABC", false), slack.MsgOptionText("Hello, world!", false), slack.MsgOptionPostMessageParameters(params))
		if err != nil {
			logger.Printf("Error posting message: %s", err)

			// Send a response message
			api.PostEphemeral(payload.Channel.ID, payload.User.ID, slack.MsgOptionText("Error posting message", false))
		}
	}

	return
}

func handleMessageShortcut(payload *slack.InteractionCallback) (err error) {
	userID := payload.User.ID
	channelID := payload.Channel.ID

	summaryStr := fmt.Sprintf("<@%s> in <#%s>", userID, channelID)
	threadTS := payload.Message.ThreadTimestamp
	if threadTS != "" {
		summaryStr += " in thread: " + threadTS
	}

	summaryModal := summaryModalView{
		Header:  "Header",
		Summary: "Summary: " + summaryStr,
		TLDR:    "TLDR",
	}

	// Send a response message
	api := slack.New(os.Getenv("SLACK_BOT_TOKEN"))

	// // get the other messages in the thread
	// var history *slack.GetConversationRepliesResponse
	// if history, err = api.GetConversationReplies(&slack.GetConversationRepliesParameters{
	// 	ChannelID: channelID,
	// 	Timestamp: threadTS,
	// }); err != nil {
	// 	logger.Printf("Error getting conversation replies: %s", err)
	// 	return
	// }

	// check if we are a member of the channel
	var channel *slack.Channel
	if channel, err = api.GetConversationInfo(&slack.GetConversationInfoInput{
		ChannelID: channelID,
	}); err != nil {
		logger.Printf("Error getting conversation info: %s", err)
		return
	}

	// if we are not a member of the channel, invite the bot
	if !channel.IsMember {
		if _, err = api.InviteUsersToConversation(channelID, os.Getenv("SLACK_BOT_USER_ID")); err != nil {
			logger.Printf("Error inviting bot to channel: %s", err)
			return
		}
	}

	// if threadTS == "" {
	// get the next message in the channel
	var history *slack.GetConversationHistoryResponse
	history, err = api.GetConversationHistory(&slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Limit:     20,
	})
	if err != nil {
		logger.Printf("Error getting conversation history: %s", err)
		return
	}

	if len(history.Messages) > 0 {
		for _, msg := range history.Messages {
			if msg.ThreadTimestamp != "" {
				pp.Println(msg.ThreadTimestamp)

				// get the thread
				threadMessages, _, _, err := api.GetConversationReplies(&slack.GetConversationRepliesParameters{
					ChannelID: channelID,
					Timestamp: msg.ThreadTimestamp,
				})
				if err != nil {
					logger.Printf("Error getting conversation replies: %s", err)
					return err
				}

				for _, threadMsg := range threadMessages {
					user := threadMsg.User
					if threadMsg.BotID != "" {
						user = threadMsg.BotID
					}

					summaryModal.Summary += fmt.Sprintf("\n<@%s>: %s", user, threadMsg.Text)
					// summary.Summary += "\n" + user + ": " + threadMsg.Text
				}
			}

			user := msg.User
			if msg.BotID != "" {
				// user = msg.BotID
			}

			summaryModal.Summary += fmt.Sprintf("\n<@%s>: %s", user, msg.Text)
		}
	}
	// }

	// options := []slack.MsgOption{
	// 	slack.MsgOptionBlocks(blocks),
	// }

	// threadTS := payload.Message.ThreadTimestamp
	// if threadTS != "" {
	// 	options = append(options, slack.MsgOptionTS(threadTS))
	// }

	view, err := summaryModal.View()
	if err != nil {
		return
	}

	_, err = api.OpenView(payload.TriggerID, view)
	if err != nil {
		logger.Printf("Error opening view: %s", err)
		return
	}

	// var s string
	// if s, err = api.PostEphemeral(channelID, userID, options...); err != nil {
	// 	logger.Printf("Error sending message: %s", err)
	// 	pp.Println(s)
	// 	return
	// }

	// if _, _, err = api.PostMessage(channelID, options...); err != nil {
	// 	logger.Printf("Error sending message: %s", err)
	// 	return
	// }

	return
}
