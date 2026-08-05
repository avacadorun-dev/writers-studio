package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"writers.studio/internal/domain"
)

var ErrUnauthorized = errors.New("invalid credentials")

func (s *Store) Authenticate(
	ctx context.Context,
	email string,
	password string,
	ttl time.Duration,
	register bool,
) (domain.User, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	var user domain.User
	var passwordHash string
	err := s.Pool.QueryRow(ctx, `
		SELECT id::text, email, password_hash, display_name, created_at
		FROM users
		WHERE email=$1
	`, email).Scan(&user.ID, &user.Email, &passwordHash, &user.DisplayName, &user.CreatedAt)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) || !register {
			return domain.User{}, "", ErrUnauthorized
		}
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if hashErr != nil {
			return domain.User{}, "", fmt.Errorf("hash password: %w", hashErr)
		}
		user = domain.User{
			ID:          uuid.NewString(),
			Email:       email,
			DisplayName: displayNameFromEmail(email),
		}
		err = s.Pool.QueryRow(ctx, `
			INSERT INTO users(id, email, password_hash, display_name)
			VALUES($1, $2, $3, $4)
			RETURNING created_at
		`, user.ID, user.Email, string(hash), user.DisplayName).Scan(&user.CreatedAt)
		if err != nil {
			return domain.User{}, "", fmt.Errorf("create user: %w", err)
		}
	} else if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return domain.User{}, "", ErrUnauthorized
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return domain.User{}, "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	tokenHash := hashToken(token)

	_, err = s.Pool.Exec(ctx, `
		INSERT INTO auth_sessions(id, user_id, token_hash, expires_at)
		VALUES($1, $2, $3, $4)
	`, uuid.NewString(), user.ID, tokenHash, time.Now().Add(ttl))
	if err != nil {
		return domain.User{}, "", fmt.Errorf("create auth session: %w", err)
	}

	return user, token, nil
}

func (s *Store) UserByToken(ctx context.Context, token string) (domain.User, error) {
	var user domain.User
	err := s.Pool.QueryRow(ctx, `
		SELECT u.id::text, u.email, u.display_name, u.created_at
		FROM auth_sessions s
		JOIN users u ON u.id=s.user_id
		WHERE s.token_hash=$1 AND s.expires_at > now()
	`, hashToken(token)).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ErrUnauthorized
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("load auth session: %w", err)
	}
	return user, nil
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	if _, err := s.Pool.Exec(ctx, "DELETE FROM auth_sessions WHERE token_hash=$1", hashToken(token)); err != nil {
		return fmt.Errorf("delete auth session: %w", err)
	}
	return nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func displayNameFromEmail(email string) string {
	name, _, _ := strings.Cut(email, "@")
	if name == "" {
		return "Автор"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
