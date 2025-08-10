package webauthnhandler

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/go-webauthn/webauthn/webauthn"
)

func (h *WebAuthnHandler) upsertUser(ctx context.Context, user webauthn.User) error {
	var err error
	stmt := `INSERT INTO users (id, display_name)
VALUES (:id, :display_name)
ON CONFLICT (id) DO UPDATE SET display_name = :display_name`
	if _, err = h.database.ReadWrite.ExecContext(ctx, stmt, user.WebAuthnID(), user.WebAuthnDisplayName()); err != nil {
		return fmt.Errorf("db upsert user %s (id: %s): %w",
			user.WebAuthnDisplayName(),
			hex.EncodeToString(user.WebAuthnID()),
			err)
	}
	return nil
}

func (h *WebAuthnHandler) getUser(ctx context.Context, id []byte) (*user, error) {
	var (
		err  error
		rows *sql.Rows
	)

	stmt := `SELECT id, display_name FROM users WHERE id = ?`
	var user user
	if err = h.database.ReadOnly.QueryRowContext(ctx, stmt, id).Scan(&user.id, &user.displayName); err != nil {
		return nil, fmt.Errorf("read user: %w", err)
	}

	// scan credentials
	stmt = `SELECT id,
       public_key,
       attestation_type,
       transport,
       flag_user_present,
       flag_user_verified,
       flag_backup_eligible,
       flag_backup_state,
       authenticator_aaguid,
       authenticator_sign_count,
       authenticator_clone_warning,
       authenticator_attachment
FROM credentials
WHERE user_id = ?`
	if rows, err = h.database.ReadOnly.QueryContext(ctx, stmt, id); err != nil {
		return nil, fmt.Errorf("query credentials: %w", err)
	}
	defer func() {
		err = rows.Close()
		if err != nil {
			h.logger.Error("could not close rows", "err", fmt.Errorf("close rows: %w", err))
		}
	}()

	for rows.Next() {
		var (
			credential webauthn.Credential
			transport  []byte
		)
		if err = rows.Scan(
			&credential.ID,
			&credential.PublicKey,
			&credential.AttestationType,
			&transport,
			&credential.Flags.UserPresent,
			&credential.Flags.UserVerified,
			&credential.Flags.BackupEligible,
			&credential.Flags.BackupState,
			&credential.Authenticator.AAGUID,
			&credential.Authenticator.SignCount,
			&credential.Authenticator.CloneWarning,
			&credential.Authenticator.Attachment,
		); err != nil {
			return nil, fmt.Errorf("scan credential: %w", err)
		}
		if err = json.Unmarshal(transport, &credential.Transport); err != nil {
			return nil, fmt.Errorf("JSON decode transport: %w", err)
		}
		user.credentials = append(user.credentials, credential)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("check rows error: %w", err)
	}

	return &user, nil
}

func (h *WebAuthnHandler) upsertCredential(ctx context.Context, userID []byte, credential *webauthn.Credential) error {
	var err error
	stmt := `INSERT INTO credentials (id,
                         user_id,
                         public_key,
                         attestation_type,
                         transport,
                         flag_user_present,
                         flag_user_verified,
                         flag_backup_eligible,
                         flag_backup_state,
                         authenticator_aaguid,
                         authenticator_sign_count,
                         authenticator_clone_warning,
                         authenticator_attachment)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (id) DO UPDATE SET attestation_type            = EXCLUDED.attestation_type,
                               transport                   = EXCLUDED.transport,
                               flag_user_present           = EXCLUDED.flag_user_present,
                               flag_user_verified          = EXCLUDED.flag_user_verified,
                               flag_backup_eligible        = EXCLUDED.flag_backup_eligible,
                               flag_backup_state           = EXCLUDED.flag_backup_state,
                               authenticator_aaguid        = EXCLUDED.authenticator_aaguid,
                               authenticator_sign_count    = EXCLUDED.authenticator_sign_count,
                               authenticator_clone_warning = EXCLUDED.authenticator_clone_warning,
                               authenticator_attachment    = EXCLUDED.authenticator_attachment;
                                 
                                   `
	var encodedTransport []byte
	encodedTransport, err = json.Marshal(credential.Transport)
	if err != nil {
		return fmt.Errorf("JSON encode transport: %w", err)
	}
	_, err = h.database.ReadWrite.ExecContext(
		ctx,
		stmt,
		credential.ID,
		userID,
		credential.PublicKey,
		credential.AttestationType,
		string(encodedTransport),
		credential.Flags.UserPresent,
		credential.Flags.UserVerified,
		credential.Flags.BackupEligible,
		credential.Flags.BackupState,
		credential.Authenticator.AAGUID,
		credential.Authenticator.SignCount,
		credential.Authenticator.CloneWarning,
		credential.Authenticator.Attachment,
	)
	if err != nil {
		return fmt.Errorf("db upsert credential (user_id: %s, credential_id: %s): %w",
			hex.EncodeToString(userID),
			hex.EncodeToString(credential.ID),
			err)
	}
	return nil
}

type role int

const (
	roleUser role = iota
	roleAdmin
)

// getUserRole returns the role of the user or sql.ErrNoRows if the user does not exist.
func (h *WebAuthnHandler) getUserRole(ctx context.Context, userID []byte) (role, error) {
	stmt := `SELECT is_admin FROM users WHERE id = ?`
	var isAdmin bool
	if err := h.database.ReadOnly.QueryRowContext(ctx, stmt, userID).Scan(&isAdmin); err != nil {
		return roleUser, fmt.Errorf("query user role: %w", err)
	}
	if isAdmin {
		return roleAdmin, nil
	}
	return roleUser, nil
}
