package main

import (
	"database/sql"
	"math/rand"
	"strconv"
	"time"
)

type List struct {
	ID          string
	TimeCreated time.Time
	Name        string
	Items       []*Item
}

type Item struct {
	ID          string
	Description string
	Done        bool
}

type sqlModel struct {
	db  *sql.DB
	rnd *rand.Rand
}

func newSQLModel(db *sql.DB) (*sqlModel, error) {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	model := &sqlModel{db, rnd}
	_, err := model.db.Exec(`
		CREATE TABLE IF NOT EXISTS lists (
			id VARCHAR(10) NOT NULL PRIMARY KEY,
			time_created TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			name VARCHAR(255) NOT NULL
		);
		
		CREATE TABLE IF NOT EXISTS items (
			id INTEGER NOT NULL PRIMARY KEY,
			list_id INTEGER NOT NULL REFERENCES lists(id),
			time_created TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			description VARCHAR(255) NOT NULL,
		    done BOOLEAN NOT NULL DEFAULT FALSE
		);
		
		CREATE INDEX IF NOT EXISTS items_list_id ON items(list_id);
		`)
	return model, err
}

func (m *sqlModel) GetLists() ([]*List, error) {
	rows, err := m.db.Query(`
		SELECT id, name, time_created
		FROM lists
		ORDER BY time_created
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

var idChars = "bcdfghjklmnpqrstvwxyz" // just consonants to avoid spelling words

func (m *sqlModel) makeID(n int) string {
	id := make([]byte, n)
	for i := 0; i < n; i++ {
		index := m.rnd.Intn(len(idChars))
		id[i] = idChars[index]
	}
	return string(id)
}

func (m *sqlModel) CreateList(name string) (*List, error) {
	id := m.makeID(10)
	_, err := m.db.Exec(`
		INSERT INTO lists (id, name)
		VALUES (?, ?)
		`, id, name)
	if err != nil {
		return nil, err
	}
	list := &List{
		ID:   id,
		Name: name,
	}
	return list, nil
}

func (m *sqlModel) GetList(id string) (*List, error) {
	row := m.db.QueryRow(`
		SELECT id, name
		FROM lists
		WHERE id = ?
		`, id)
	var list List
	err := row.Scan(&list.ID, &list.Name)
	if err != nil {
		return nil, err
	}
	list.Items, err = m.getListItems(id)
	return &list, err
}

func (m *sqlModel) getListItems(listID string) ([]*Item, error) {
	rows, err := m.db.Query(`
		SELECT id, description, done
		FROM items
		WHERE list_id = ?
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

func (m *sqlModel) AddItem(listID, description string) (*Item, error) {
	result, err := m.db.Exec(`
		INSERT INTO items (list_id, description)
		VALUES (?, ?)
		`, listID, description)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	item := &Item{
		ID:          strconv.Itoa(int(id)),
		Description: description,
	}
	return item, nil
}

func (m *sqlModel) CheckItem(listID, itemID string, done bool) error {
	_, err := m.db.Exec(`
		UPDATE items
		SET done = ?
		WHERE list_id = ? AND id = ?
		`, done, listID, itemID)
	return err
}

func (m *sqlModel) DeleteItem(listID, itemID string) error {
	_, err := m.db.Exec(`
		DELETE FROM items
		WHERE list_id = ? AND id = ?
		`, listID, itemID)
	return err
}
