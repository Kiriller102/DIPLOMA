package repository

import "github.com/jmoiron/sqlx"

// LockChat serializes message recipients with changes to chat membership.
func LockChat(tx *sqlx.Tx, chatID int) error {
	var id int
	return tx.Get(&id, `SELECT id FROM chats WHERE id=$1 FOR UPDATE`, chatID)
}
