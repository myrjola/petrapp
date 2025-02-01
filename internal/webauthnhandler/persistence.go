package webauthnhandler

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/myrjola/petrapp/internal/errors"
	"log/slog"
)

func (h *WebAuthnHandler) upsertUser(ctx context.Context, user webauthn.User) error {
	var err error
	stmt := `INSERT INTO users (id, display_name)
VALUES (:id, :display_name)
ON CONFLICT (id) DO UPDATE SET display_name = :display_name`
	if _, err = h.database.ReadWrite.ExecContext(ctx, stmt, user.WebAuthnID(), user.WebAuthnDisplayName()); err != nil {
		return errors.Wrap(
			err,
			"db upsert",
			slog.String("display_name", user.WebAuthnDisplayName()),
			slog.Any("user_id", hex.EncodeToString(user.WebAuthnID())),
		)
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
		return nil, errors.Wrap(err, "read user")
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
		return nil, errors.Wrap(err, "query credentials")
	}
	defer func() {
		err = rows.Close()
		if err != nil {
			h.logger.Error("could not close rows", "err", errors.Wrap(err, "close rows"))
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
			return nil, errors.Wrap(err, "scan credential")
		}
		if err = json.Unmarshal(transport, &credential.Transport); err != nil {
			return nil, errors.Wrap(err, "JSON decode transport")
		}
		user.credentials = append(user.credentials, credential)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.Wrap(err, "check rows error")
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
		return errors.Wrap(err, "JSON encode transport")
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
		return errors.Wrap(err, "db upsert credential",
			slog.String("user_id", hex.EncodeToString(userID)),
			slog.String("credential_id", hex.EncodeToString(credential.ID)),
		)
	}
	return nil
}

func (h *WebAuthnHandler) userExists(ctx context.Context, userID []byte) (bool, error) {
	stmt := `SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)`
	var exists bool
	if err := h.database.ReadOnly.QueryRowContext(ctx, stmt, userID).Scan(&exists); err != nil {
		return false, errors.Wrap(err, "query user exists")
	}
	return exists, nil
}
