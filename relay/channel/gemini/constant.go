package gemini

var ModelList = []string{ // stable version
	"gemini-2.5-flash", "gemini-2.5-pro",
	"gemini-2.5-flash-lite",
	"gemini-3-pro-image", "gemini-3.1-flash-image",
	// latest version
	"gemini-flash-latest", "gemini-flash-lite-latest", "gemini-pro-latest",
	"gemini-2.5-flash-native-audio-latest",
	// preview version
	"gemini-2.5-flash-preview-tts", "gemini-2.5-pro-preview-tts",
	"gemini-2.5-flash-image",
	"gemini-3-flash-preview", "gemini-3.1-pro-preview",
	"gemini-3.1-pro-preview-customtools",
	"nano-banana-pro-preview",
	"gemini-2.5-flash-native-audio-preview-09-2025", "gemini-2.5-flash-native-audio-preview-12-2025",
	// gemma models
	"gemma-3-1b-it", "gemma-3-4b-it", "gemma-3-12b-it",
	"gemma-3-27b-it", "gemma-3n-e4b-it", "gemma-3n-e2b-it",
	// embedding models
	"gemini-embedding-001", "gemini-embedding-2-preview",
	// imagen models
	// veo models
	// other models
	"aqa",
	// Current provider catalogue; account and regional access still apply.
	"gemini-3.1-flash-lite",
	"gemini-3.1-flash-lite-image",
	"gemini-3.5-flash-lite",
	"gemini-3.6-flash",
	"gemini-3.8-flash",
	"gemini-4-argon",
	"gemini-embedding-2",
	"gemini-nano-banana-2.1",
	"gemini-robotics-er-2-preview",
	// Speech models use native Gemini generateContent with audio modalities.
	"gemini-3.8-flash-lite-tts",
	"gemini-3.8-flash-tts",
}

var SafetySettingList = []string{
	"HARM_CATEGORY_HARASSMENT",
	"HARM_CATEGORY_HATE_SPEECH",
	"HARM_CATEGORY_SEXUALLY_EXPLICIT",
	"HARM_CATEGORY_DANGEROUS_CONTENT",
	//"HARM_CATEGORY_CIVIC_INTEGRITY", This item is deprecated!
}

var ChannelName = "google gemini"
