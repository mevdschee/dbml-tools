package generator

import (
	"dbml-tools/interpreter"
	"fmt"
	"strings"
)

// escDot escapes a string for use in a Graphviz HTML label.
func escDot(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// dotID returns a valid DOT node identifier for a table name.
func dotID(name string) string {
	r := strings.NewReplacer(
		" ", "_", "-", "_", ".", "_",
		"å", "a", "ä", "a", "ö", "o", "ü", "u",
		"æ", "ae", "ø", "o", "Å", "A", "Ä", "A",
		"Ö", "O", "Ü", "U", "Æ", "AE", "Ø", "O",
	)
	id := r.Replace(name)
	// Remove any remaining non-alphanumeric/underscore chars
	var sb strings.Builder
	for _, ch := range id {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

// Dot generates a Graphviz DOT representation of the database schema.
func Dot(db *interpreter.Database) string {
	var sb strings.Builder

	sb.WriteString("digraph erd {\n")
	sb.WriteString("    graph [\n")
	sb.WriteString("        rankdir=LR\n")
	sb.WriteString("        bgcolor=\"#ffffff\"\n")
	sb.WriteString("        fontname=\"Helvetica\"\n")
	sb.WriteString("        pad=0.5\n")
	sb.WriteString("        nodesep=0.8\n")
	sb.WriteString("        ranksep=1.5\n")
	sb.WriteString("        splines=ortho\n")
	sb.WriteString("    ];\n")
	sb.WriteString("    node [\n")
	sb.WriteString("        shape=plain\n")
	sb.WriteString("        fontname=\"Helvetica\"\n")
	sb.WriteString("        fontsize=12\n")
	sb.WriteString("    ];\n")
	sb.WriteString("    edge [\n")
	sb.WriteString("        color=\"#4a4a4a\"\n")
	sb.WriteString("        penwidth=1.2\n")
	sb.WriteString("        fontname=\"Helvetica\"\n")
	sb.WriteString("        fontsize=9\n")
	sb.WriteString("    ];\n\n")

	// Build a set of FK columns per table for visual marking
	fkCols := make(map[string]map[string]bool)
	for _, ref := range db.Refs {
		if len(ref.Endpoints) != 2 {
			continue
		}
		ep0, ep1 := ref.Endpoints[0], ref.Endpoints[1]
		var child interpreter.RefEndpoint
		switch {
		case ep0.Relation == "*" && ep1.Relation == "1":
			child = ep0
		case ep0.Relation == "1" && ep1.Relation == "*":
			child = ep1
		case ep0.Relation == "1" && ep1.Relation == "1":
			child = ep0
		default:
			continue
		}
		if fkCols[child.TableName] == nil {
			fkCols[child.TableName] = make(map[string]bool)
		}
		for _, f := range child.FieldNames {
			fkCols[child.TableName][f] = true
		}
	}

	// Emit table nodes
	for _, tbl := range db.Tables {
		id := dotID(tbl.Name)
		sb.WriteString(fmt.Sprintf("    %s [label=<\n", id))
		sb.WriteString("        <TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"6\">\n")

		// Header row
		sb.WriteString(fmt.Sprintf(
			"            <TR><TD COLSPAN=\"3\" BGCOLOR=\"#4a86c8\" ALIGN=\"CENTER\"><FONT COLOR=\"#ffffff\"><B>%s</B></FONT></TD></TR>\n",
			escDot(tbl.Name),
		))

		// Column rows
		for _, col := range tbl.Fields {
			args := typeArgs(col)
			sqlType, _ := mapType(col.Type.TypeName, args, Generic, db)

			// Key indicator
			keyIcon := " "
			if col.PK {
				keyIcon = "PK"
			} else if fkCols[tbl.Name] != nil && fkCols[tbl.Name][col.Name] {
				keyIcon = "FK"
			}

			// Constraint badges
			var constraints []string
			if col.NotNull != nil && *col.NotNull {
				constraints = append(constraints, "NN")
			}
			if col.Unique {
				constraints = append(constraints, "UQ")
			}

			keyColor := "#e8e8e8"
			if col.PK {
				keyColor = "#f0d080"
			} else if keyIcon == "FK" {
				keyColor = "#d0e8f0"
			}

			constraintStr := strings.Join(constraints, " ")

			sb.WriteString(fmt.Sprintf(
				"            <TR><TD BGCOLOR=\"%s\" ALIGN=\"LEFT\" PORT=\"%s\"><FONT POINT-SIZE=\"10\">%s</FONT></TD><TD ALIGN=\"LEFT\">%s</TD><TD ALIGN=\"LEFT\"><FONT POINT-SIZE=\"9\" COLOR=\"#888888\">%s</FONT></TD></TR>\n",
				keyColor,
				escDot(col.Name),
				escDot(keyIcon),
				escDot(col.Name),
				escDot(sqlType+" "+constraintStr),
			))
		}

		// Table note
		if tbl.Note != nil && tbl.Note.Value != "" {
			sb.WriteString(fmt.Sprintf(
				"            <TR><TD COLSPAN=\"3\" BGCOLOR=\"#f5f5f5\" ALIGN=\"LEFT\"><FONT POINT-SIZE=\"9\" COLOR=\"#666666\"><I>%s</I></FONT></TD></TR>\n",
				escDot(tbl.Note.Value),
			))
		}

		sb.WriteString("        </TABLE>\n")
		sb.WriteString("    >];\n\n")
	}

	// Emit relationships
	for _, ref := range db.Refs {
		if len(ref.Endpoints) != 2 {
			continue
		}
		ep0, ep1 := ref.Endpoints[0], ref.Endpoints[1]

		var child, parent interpreter.RefEndpoint
		var label string
		switch {
		case ep0.Relation == "*" && ep1.Relation == "1":
			child, parent = ep0, ep1
			label = "*:1"
		case ep0.Relation == "1" && ep1.Relation == "*":
			child, parent = ep1, ep0
			label = "*:1"
		case ep0.Relation == "1" && ep1.Relation == "1":
			child, parent = ep0, ep1
			label = "1:1"
		default:
			continue
		}

		if len(child.FieldNames) == 0 || len(parent.FieldNames) == 0 {
			continue
		}

		arrowhead := "crowodot"
		if label == "1:1" {
			arrowhead = "teeodot"
		}

		sb.WriteString(fmt.Sprintf(
			"    %s -> %s [arrowhead=%s arrowtail=teeodot dir=both xlabel=\"%s\"];\n",
			dotID(child.TableName),
			dotID(parent.TableName),
			arrowhead, label,
		))
	}

	sb.WriteString("}\n")
	return sb.String()
}
