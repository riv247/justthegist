package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
)

type responseStruct struct {
	PromptVersion string      `json:"prompt_version"`
	Context       interface{} `json:"context"`
	Summary       string      `json:"summary"`
	Tldr          string      `json:"tldr"`
}

func main() {
	promptVersion := "0.0.1"

	responses := []responseStruct{}
	numResponses := rand.Intn(4) + 2 // Generate 2-5 responses

	for i := 0; i < numResponses; i++ {
		response := responseStruct{
			PromptVersion: promptVersion,
			Context:       nil,
			Summary:       "This is a sample summary " + strconv.Itoa(i+1),
			Tldr:          "This is a sample TLDR " + strconv.Itoa(i+1),
		}

		responses = append(responses, response)
	}

	b, _ := json.Marshal(responses)
	fmt.Println(string(b))
}
