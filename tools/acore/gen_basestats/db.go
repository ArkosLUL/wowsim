package main

import (
	"database/sql"
	"fmt"
)

// primaryStats is a player_class_stats or player_race_stats row.
type primaryStats struct {
	Strength, Agility, Stamina, Intellect, Spirit int32
}

type classStats struct {
	BaseHP, BaseMana int32
	primaryStats
}

func loadClassStats(db *sql.DB, level int) (map[int]classStats, error) {
	rows, err := db.Query(`SELECT Class, BaseHP, BaseMana, Strength, Agility, Stamina, Intellect, Spirit
		FROM player_class_stats WHERE Level = ?`, level)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int]classStats{}
	for rows.Next() {
		var id int
		var s classStats
		if err := rows.Scan(&id, &s.BaseHP, &s.BaseMana, &s.Strength, &s.Agility, &s.Stamina, &s.Intellect, &s.Spirit); err != nil {
			return nil, err
		}
		out[id] = s
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, c := range classes {
		if _, ok := out[c.serverID]; !ok {
			return nil, fmt.Errorf("player_class_stats has no level %d row for CLASS_%s", level, c.name)
		}
	}
	return out, nil
}

func loadRaceStats(db *sql.DB) (map[int]primaryStats, error) {
	rows, err := db.Query(`SELECT Race, Strength, Agility, Stamina, Intellect, Spirit FROM player_race_stats`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int]primaryStats{}
	for rows.Next() {
		var id int
		var s primaryStats
		if err := rows.Scan(&id, &s.Strength, &s.Agility, &s.Stamina, &s.Intellect, &s.Spirit); err != nil {
			return nil, err
		}
		out[id] = s
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, r := range races {
		if _, ok := out[r.serverID]; !ok {
			return nil, fmt.Errorf("player_race_stats has no row for race %d (%s)", r.serverID, r.proto)
		}
	}
	return out, nil
}
