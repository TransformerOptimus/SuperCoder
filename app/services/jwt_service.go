package services

import (
	"errors"
	"fmt"
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

func (s *JWTService) GenerateTokenForInvite(organisationId int, userEmail string) (string, error) {
	fmt.Println("Inside generate token for invite")
	jwtKey := []byte(s.secretKey)
	var expiryTimeHours = s.getJWTExpiryHours()

	data := map[string]interface{}{
		"user_email":      userEmail,
		"organisation_id": organisationId,
	}

	var tokenString, err = s.getTokenString(data, expiryTimeHours, jwtKey)
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

func (s *JWTService) DecodeInviteToken(tokenString string) (string, int, error) {
	token, err := s.ValidateToken(tokenString)
	if err != nil {
		return "", 0, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		email, ok := claims["user_email"].(string)
		if !ok {
			return "", 0, errors.New("invalid email in token")
		}

		organizationId, ok := claims["organisation_id"].(float64)
		if !ok {
			return "", 0, errors.New("invalid organisation_id in token")
		}

		return email, int(organizationId), nil
	}
	return "", 0, errors.New("invalid token")
}

func (s *JWTService) getJWTExpiryHours() time.Duration {
	expiryTimeHours := s.jwtExpiryHours
	if expiryTimeHours == 0 {
		expiryTimeHours = 200
	}
	return expiryTimeHours
}

func (s *JWTService) getTokenString(data map[string]interface{}, expiryTimeHours time.Duration, jwtKey []byte) (string, error) {
	toEncode := make(jwt.MapClaims)
	for key, value := range data {
		toEncode[key] = value
	}

	expire := time.Now().Add(expiryTimeHours)
	toEncode["exp"] = expire.Unix()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, toEncode)
	return token.SignedString(jwtKey)
}

func NewJwtService(secretKey string, jwtExpiryHours time.Duration) *JWTService {
	return &JWTService{
		secretKey:      secretKey,
		jwtExpiryHours: jwtExpiryHours,
	}
}
