package azerothcore

import (
	"database/sql"
	"fmt"
	"io"
	"strings"
)

// BisTablesSQL creates mod-bis-tooltip's tables in acore_world. The module ships the same DDL; keep
// the two in step.
const BisTablesSQL = "CREATE TABLE IF NOT EXISTS `bistooltip_dataset` (\n" +
	"  `version` VARCHAR(16) NOT NULL,\n" +
	"  `sim_commit` VARCHAR(64) NOT NULL,\n" +
	"  `catalog_date` VARCHAR(10) NOT NULL,\n" +
	"  `objective` VARCHAR(16) NOT NULL,\n" +
	"  `fingerprint` VARCHAR(8) NOT NULL,\n" +
	"  `exported_at` DATETIME NOT NULL,\n" +
	"  PRIMARY KEY (`version`)\n" +
	") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='BiS tooltip dataset, one row';\n" +
	"\n" +
	"CREATE TABLE IF NOT EXISTS `bistooltip_subject` (\n" +
	"  `id` SMALLINT UNSIGNED NOT NULL,\n" +
	"  `kind` TINYINT UNSIGNED NOT NULL COMMENT '0 roster raider, 1 class spec',\n" +
	"  `guid` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `name` VARCHAR(12) NOT NULL DEFAULT '',\n" +
	"  `class_id` TINYINT UNSIGNED NOT NULL,\n" +
	"  `spec_name` VARCHAR(32) NOT NULL,\n" +
	"  `raid_index` TINYINT NOT NULL DEFAULT -1,\n" +
	"  PRIMARY KEY (`id`)\n" +
	") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='BiS tooltip subjects';\n" +
	"\n" +
	"CREATE TABLE IF NOT EXISTS `bistooltip_block` (\n" +
	"  `subject_id` SMALLINT UNSIGNED NOT NULL,\n" +
	"  `content_phase` TINYINT UNSIGNED NOT NULL,\n" +
	"  `payload` TEXT NOT NULL,\n" +
	"  `checksum` CHAR(8) NOT NULL,\n" +
	"  PRIMARY KEY (`subject_id`, `content_phase`)\n" +
	") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='BiS tooltip blocks, id = subject_id * 10 + content_phase';\n"

// WriteBisSQL writes the dataset as an import for acore_world. It replaces whatever dataset is there,
// in one transaction, and creates the tables if the module hasn't yet.
func WriteBisSQL(w io.Writer, dataset *BisDataset) error {
	var sb strings.Builder
	fmt.Fprintf(&sb, "-- BiS tooltip dataset %s: sim %s, catalog %s, objective %s, %d subjects, %d blocks\n\n",
		dataset.Version, dataset.SimCommit, dataset.CatalogDate, dataset.Objective, len(dataset.Subjects), len(dataset.Blocks))
	sb.WriteString(BisTablesSQL)
	sb.WriteString("\nSTART TRANSACTION;\n")
	sb.WriteString("DELETE FROM `bistooltip_block`;\n")
	sb.WriteString("DELETE FROM `bistooltip_subject`;\n")
	sb.WriteString("DELETE FROM `bistooltip_dataset`;\n\n")

	fmt.Fprintf(&sb, "INSERT INTO `bistooltip_dataset` (`version`, `sim_commit`, `catalog_date`, `objective`, `fingerprint`, `exported_at`) VALUES\n(%s, %s, %s, %s, %s, %s);\n\n",
		sqlString(dataset.Version), sqlString(dataset.SimCommit), sqlString(dataset.CatalogDate), sqlString(dataset.Objective),
		sqlString(dataset.Fingerprint), sqlString(dataset.ExportedAt.UTC().Format("2006-01-02 15:04:05")))

	sb.WriteString("INSERT INTO `bistooltip_subject` (`id`, `kind`, `guid`, `name`, `class_id`, `spec_name`, `raid_index`) VALUES\n")
	for i, s := range dataset.Subjects {
		fmt.Fprintf(&sb, "(%d, %d, %d, %s, %d, %s, %d)%s\n", s.ID, s.Kind, s.GUID, sqlString(s.Name), s.ClassID,
			sqlString(s.SpecName), s.RaidIndex, sqlRowEnd(i, len(dataset.Subjects)))
	}

	sb.WriteString("\nINSERT INTO `bistooltip_block` (`subject_id`, `content_phase`, `payload`, `checksum`) VALUES\n")
	for i, b := range dataset.Blocks {
		fmt.Fprintf(&sb, "(%d, %d, %s, %s)%s\n", b.SubjectID, b.ContentPhase, sqlString(b.Payload), sqlString(b.Checksum),
			sqlRowEnd(i, len(dataset.Blocks)))
	}
	sb.WriteString("\nCOMMIT;\n")

	_, err := io.WriteString(w, sb.String())
	return err
}

func sqlRowEnd(i, n int) string {
	if i == n-1 {
		return ";"
	}
	return ","
}

// sqlString quotes s for MySQL's default sql_mode, where a backslash escapes.
func sqlString(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `''`, "\n", `\n`, "\r", `\r`, "\x00", `\0`).Replace(s) + "'"
}

// CharacterGUIDs looks characters up by name. Every name has to exist.
func CharacterGUIDs(db *sql.DB, names []string) (map[string]uint32, error) {
	members, err := selectNamedMembers(db, names)
	if err != nil {
		return nil, err
	}
	guids := make(map[string]uint32, len(names))
	for i, member := range members {
		guids[names[i]] = member.guid
	}
	return guids, nil
}
