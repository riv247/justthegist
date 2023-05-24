package ai

import (
	"encoding/json"
	"log"
	"os"
)

type PromptParams map[string]interface{}

type PromptCommonStruct struct {
	Provider      string `json:"provider,omitempty"`
	ProviderID    string `json:"provider_id,omitempty"`
	PromptVersion string `json:"v,omitempty"`
}

type PromptReqStruct struct {
	PromptCommonStruct                // `json:"prompt_common_struct,omitempty"`
	Params               PromptParams `json:"p,omitempty"`
	OptionalInstructions string       `json:"oi,omitempty"`
	Text                 string       `json:"text,omitempty"`
}

func (promptReq PromptReqStruct) JSON() (b []byte, err error) {
	b, err = json.Marshal(promptReq)
	return
}

type PromptResStruct struct {
	PromptCommonStruct          // `json:"prompt_common_struct,omitempty"`
	Context            string   `json:"context,omitempty"`
	Summary            []string `json:"summary,omitempty"`
	TLDR               string   `json:"tldr,omitempty"`
}

var (
	SummarizePrompt string
)

func init() {
	logger = log.New(os.Stdout, "[PROMPT] ", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)
}
