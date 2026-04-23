package http

const (
	// AccessTokenCookie used as access token cookie name and response body field.
	// Value based on GenerateJWT
	AccessTokenCookie = "access_token"
	// RefreshTokenCookie used as refresh token cookie name and response body field.
	// Value based on Session.ID
	RefreshTokenCookie = "refresh_token"

	// ImageFormKey is universal form-data key for menu and user profile image
	ImageFormKey = "image"
)
