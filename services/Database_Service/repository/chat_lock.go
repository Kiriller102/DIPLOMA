package repository

import (
	"context"
	"github.com/jmoiron/sqlx"
)

// LockChat serializes message recipients with changes to chat membership.
func LockChat(ctx context.Context, tx *sqlx.Tx, chatID int) error {
	var id int
	return tx.GetContext(ctx, &id, `SELECT id FROM chats WHERE id=$1 FOR UPDATE`, chatID)
}
