package oauth

// OAuthToken represents the token received from OAuth provider
type OAuthToken struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

// OAuthUser represents the user info from OAuth provider
type OAuthUser struct {
	// ProviderUserID is the unique identifier from the OAuth provider
	ProviderUserID string
	// Username is the username from the OAuth provider (e.g., GitHub login)
	Username string
	// DisplayName is the display name from the OAuth provider
	DisplayName string
	// Email is the email from the OAuth provider
	Email string
	// Extra contains any additional provider-specific data
	Extra map[string]any
}
