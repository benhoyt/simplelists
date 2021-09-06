package main

import (
	"database/sql"
	"math/rand"
	"strconv"
	"time"
)

// List is a to-do list (along with its list items).
type List struct {
	ID          string
	TimeCreated time.Time
	Name        string
	Items       []*Item
}

// Item is a single to-do list item.
type Item struct {
	ID          string
	Description string
	Done        bool
}

// SQLModel represents the database query model implemented with SQLite.
type SQLModel struct {
	db  *sql.DB
	rnd *rand.Rand
}

// NewSQLModel returns a new SQLite database model, creating tables if they
// don't already exist.
func NewSQLModel(db *sql.DB) (*SQLModel, error) {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	model := &SQLModel{db, rnd}
	_, err := model.db.Exec(`
		CREATE TABLE IF NOT EXISTS lists (
			id VARCHAR(10) NOT NULL PRIMARY KEY,
			time_created TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			name VARCHAR(255) NOT NULL,
		    time_deleted TIMESTAMP
		);
		
		CREATE TABLE IF NOT EXISTS items (
			id INTEGER NOT NULL PRIMARY KEY,
			list_id INTEGER NOT NULL REFERENCES lists(id),
			time_created TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			description VARCHAR(255) NOT NULL,
		    done BOOLEAN NOT NULL DEFAULT FALSE,
		    time_deleted TIMESTAMP
		);
		
		CREATE INDEX IF NOT EXISTS items_list_id ON items(list_id);
		`)
	return model, err
}

// GetLists fetches all the to-do lists (without their items), ordered with
// the most recent first.
func (m *SQLModel) GetLists() ([]*List, error) {
	rows, err := m.db.Query(`
		SELECT id, name, time_created
		FROM lists
		WHERE time_deleted IS NULL
		ORDER BY time_created DESC
		`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lists []*List
	for rows.Next() {
		var list List
		err = rows.Scan(&list.ID, &list.Name, &list.TimeCreated)
		if err != nil {
			return nil, err
		}
		lists = append(lists, &list)
	}
	return lists, rows.Err()
}

var listIDChars = "bcdfghjklmnpqrstvwxyz" // just consonants to avoid spelling words

// makeListID creates a new randomized list ID.
func (m *SQLModel) makeListID(n int) string {
	id := make([]byte, n)
	for i := 0; i < n; i++ {
		index := m.rnd.Intn(len(listIDChars))
		id[i] = listIDChars[index]
	}
	return string(id)
}

// CreateList creates a new list with the given name, returning its ID.
func (m *SQLModel) CreateList(name string) (string, error) {
	id := m.makeListID(10)
	_, err := m.db.Exec("INSERT INTO lists (id, name) VALUES (?, ?)", id, name)
	return id, err
}

// DeleteList (soft) deletes the given list (its items actually remain
// untouched). It's not an error if the list doesn't exist.
func (m *SQLModel) DeleteList(id string) error {
	_, err := m.db.Exec("UPDATE lists SET time_deleted = CURRENT_TIMESTAMP WHERE id = ?", id)
	return err
}

// GetList fetches a single list and returns the List, or nil if not found.
func (m *SQLModel) GetList(id string) (*List, error) {
	row := m.db.QueryRow("SELECT id, name FROM lists WHERE id = ? AND time_deleted IS NULL", id)
	var list List
	err := row.Scan(&list.ID, &list.Name)
	if err != nil {
		return nil, err
	}
	list.Items, err = m.getListItems(id)
	return &list, err
}

func (m *SQLModel) getListItems(listID string) ([]*Item, error) {
	rows, err := m.db.Query(`
		SELECT id, description, done
		FROM items
		WHERE list_id = ? AND time_deleted IS NULL
		ORDER BY time_created
		`, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*Item
	for rows.Next() {
		var item Item
		err = rows.Scan(&item.ID, &item.Description, &item.Done)
		if err != nil {
			return nil, err
		}
		items = append(items, &item)
	}
	return items, rows.Err()
}

// AddItem adds an item with the given description to a list, returning the
// item ID.
func (m *SQLModel) AddItem(listID, description string) (string, error) {
	result, err := m.db.Exec("INSERT INTO items (list_id, description) VALUES (?, ?)",
		listID, description)
	if err != nil {
		return "", err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return "", err
	}
	return strconv.Itoa(int(id)), nil
}

// UpdateDone updates the "done" flag of the given item in a list.
func (m *SQLModel) UpdateDone(listID, itemID string, done bool) error {
	_, err := m.db.Exec("UPDATE items SET done = ? WHERE list_id = ? AND id = ?",
		done, listID, itemID)
	return err
}

// DeleteItem (soft) deletes the given item in a list.
func (m *SQLModel) DeleteItem(listID, itemID string) error {
	_, err := m.db.Exec(`
			UPDATE items
			SET time_deleted = CURRENT_TIMESTAMP
			WHERE list_id = ? AND id = ?
		`, listID, itemID)
	return err
}
