package cohere

import "github.com/QuantumNous/new-api/relaykit/dto"

type CohereRequest struct {
	Model       string        `json:"model"`
	ChatHistory []ChatHistory `json:"chat_history"`
	Message     string        `json:"message"`
	Stream      bool          `json:"stream"`
	MaxTokens   uint          `json:"max_tokens"`
	SafetyMode  string        `json:"safety_mode,omitempty"`
}

type ChatHistory struct {
	Role    string `json:"role"`
	Message string `json:"message"`
}

type CohereResponse struct {
	IsFinished   bool                  `json:"is_finished"`
	EventType    string                `json:"event_type"`
	Text         string                `json:"text,omitempty"`
	FinishReason string                `json:"finish_reason,omitempty"`
	Response     *CohereResponseResult `json:"response"`
}

type CohereResponseResult struct {
	ResponseId   string     `json:"response_id"`
	FinishReason string     `json:"finish_reason,omitempty"`
	Text         string     `json:"text"`
	Meta         CohereMeta `json:"meta"`
}

type CohereRerankRequest struct {
	Documents       []any  `json:"documents"`
	Query           string `json:"query"`
	Model           string `json:"model"`
	TopN            int    `json:"top_n"`
	ReturnDocuments bool   `json:"return_documents"`
}

type CohereRerankResponseResult struct {
	Results []dto.RerankResponseResult `json:"results"`
	Meta    CohereMeta                 `json:"meta"`
}

type CohereMeta struct {
	//Tokens CohereTokens `json:"tokens"`
	BilledUnits CohereBilledUnits `json:"billed_units"`
}

type CohereBilledUnits struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type CohereTokens struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type CohereEmbeddingRequest struct {
	Model           string   `json:"model"`
	Texts           []string `json:"texts"`
	InputType       string   `json:"input_type"`
	EmbeddingTypes  []string `json:"embedding_types"`
	OutputDimension *int     `json:"output_dimension,omitempty"`
}

type CohereEmbeddingResponse struct {
	Embeddings struct {
		Float  [][]*float64 `json:"float"`
		Base64 []string     `json:"base64"`
	} `json:"embeddings"`
	Meta struct {
		BilledUnits struct {
			InputTokens *float64 `json:"input_tokens"`
		} `json:"billed_units"`
		Tokens struct {
			InputTokens *float64 `json:"input_tokens"`
		} `json:"tokens"`
	} `json:"meta"`
}
