package services

import (
	"github.com/dgrijalva/jwt-go"
	"time"
)

type JWTService struct {
	secretKey      string
	jwtExpiryHours time.Duration
}

func (s *JWTService) GenerateToken(userId int, email string) (string, error) {
	jwtKey := []byte(s.secretKey)
	expiryTimeHours := s.jwtExpiryHours
	if expiryTimeHours == 0 {
		expiryTimeHours = 200
	}
	//expiresAt := time.Now().Add(time.Hour * expiryTimeHours)

	data := map[string]interface{}{
		"user_id": userId,
		"email":   email,
	}
	// Create the JWT claims, which includes the email and expiry time
	toEncode := make(jwt.MapClaims)
	for key, value := range data {
		toEncode[key] = value
	}

	expire := time.Now().Add(expiryTimeHours)
	toEncode["exp"] = expire.Unix()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, toEncode)
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

func (s *JWTService) ValidateToken(tokenString string) (*jwt.Token, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.secretKey), nil
	})
	return token, err
}

func NewJwtService(secretKey string, jwtExpiryHours time.Duration) *JWTService {
	return &JWTService{
		secretKey:      secretKey,
		jwtExpiryHours: jwtExpiryHours,
	}
}
