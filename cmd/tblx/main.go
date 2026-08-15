// Command tblx converts, inspects, browses and exports TBLX columnar files.
//
// Usage:
//
//	tblx import <file.csv>            convert CSV to TBL via an interactive wizard
//	tblx view   <file.tblx>           browse a TBL file in the terminal
//	tblx info   <file.tblx>           print file metadata
//	tblx export <file.tblx> [out.csv] convert TBL back to CSV (stdout if omitted)
package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tblx "github.com/askmehrun/libtblx"

	"github.com/askmehrun/tblx/internal/tui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "tblx:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage(os.Stderr)
		return fmt.Errorf("no command given")
	}
	switch args[0] {
	case "import":
		if len(args) != 2 {
			return fmt.Errorf("usage: tblx import <file.csv>")
		}
		return cmdImport(args[1])
	case "view":
		if len(args) != 2 {
			return fmt.Errorf("usage: tblx view <file.tblx>")
		}
		return tui.RunViewer(args[1])
	case "info":
		if len(args) != 2 {
			return fmt.Errorf("usage: tblx info <file.tblx>")
		}
		return cmdInfo(args[1])
	case "export":
		if len(args) != 2 && len(args) != 3 {
			return fmt.Errorf("usage: tblx export <file.tblx> [output.csv]")
		}
		out := ""
		if len(args) == 3 {
			out = args[2]
		}
		return cmdExport(args[1], out)
	case "help", "-h", "--help":
		usage(os.Stdout)
		return nil
	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "tblx — binary columnar files for the terminal (Tablix)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  tblx import <file.csv>            CSV → TBL, with an interactive type wizard")
	fmt.Fprintln(w, "  tblx view   <file.tblx>           scrollable table viewer (q quits)")
	fmt.Fprintln(w, "  tblx info   <file.tblx>           print metadata and schema")
	fmt.Fprintln(w, "  tblx export <file.tblx> [out.csv] TBL → CSV (writes to stdout if out.csv is omitted)")
}

// cmdImport drives the whole CSV → TBL pipeline: parse the CSV, run the
// type wizard, convert, write, report.
func cmdImport(path string) error {
	headers, rows, err := readCSV(path)
	if err != nil {
		return err
	}

	samples := rows
	if len(samples) > 3 {
		samples = samples[:3]
	}
	types, err := tui.RunImportWizard(path, headers, samples)
	if err != nil {
		return err
	}

	data, err := tblx.ConvertCSV(headers, rows, types)
	if err != nil {
		return err
	}
	w, err := tblx.NewWriter(headers, types, data)
	if err != nil {
		return err
	}

	out := strings.TrimSuffix(path, filepath.Ext(path)) + ".tblx"
	if err := w.Write(out); err != nil {
		return err
	}

	fi, err := os.Stat(out)
	if err != nil {
		return err
	}
	fmt.Printf("wrote %s — %d rows x %d cols, %s\n", out, len(rows), len(headers), humanSize(fi.Size()))
	for i, h := range headers {
		fmt.Printf("  %2d. %-16s %s\n", i+1, h, types[i])
	}
	return nil
}

// cmdInfo prints everything the header knows without reading data blocks.
func cmdInfo(path string) error {
	r, err := tblx.NewReader(path)
	if err != nil {
		return err
	}
	defer r.Close()

	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	fmt.Printf("file     %s\n", path)
	fmt.Printf("format   TBLX (magic %q)\n", tblx.Magic)
	fmt.Printf("rows     %d\n", r.NRows)
	fmt.Printf("columns  %d\n", r.NCols)
	fmt.Printf("size     %s (%d bytes)\n", humanSize(fi.Size()), fi.Size())
	fmt.Println("schema")
	for i := range r.ColNames {
		cl, _ := r.ColLen(i)
		fmt.Printf("  %2d. %-16s %-7s block %s\n", i+1, r.ColNames[i], r.ColTypes[i], humanSize(int64(cl)))
	}
	return nil
}

// cmdExport decodes the whole table and writes it back out as CSV, to
// out if given, otherwise to stdout.
func cmdExport(path, out string) error {
	r, err := tblx.NewReader(path)
	if err != nil {
		return err
	}
	defer r.Close()

	data, err := r.ReadAll()
	if err != nil {
		return err
	}
	records := make([][]string, 0, len(data)+1)
	records = append(records, r.ColNames)
	for _, row := range data {
		rec := make([]string, r.NCols)
		for ci, name := range r.ColNames {
			rec[ci] = tblx.FormatValue(row[name])
		}
		records = append(records, rec)
	}

	var w io.Writer = os.Stdout
	if out != "" {
		f, err := os.Create(out)
		if err != nil {
			return fmt.Errorf("create %s: %w", out, err)
		}
		defer f.Close()
		w = f
	}

	cw := csv.NewWriter(w)
	if err := cw.WriteAll(records); err != nil {
		return fmt.Errorf("write csv: %w", err)
	}
	if out != "" {
		fmt.Fprintf(os.Stderr, "exported %d rows x %d cols to %s\n", r.NRows, r.NCols, out)
	}
	return nil
}

// readCSV reads path, treating the first record as the header row.
func readCSV(path string) ([]string, [][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	cr := csv.NewReader(f)
	cr.FieldsPerRecord = -1 // ragged rows are tolerated; missing cells read as ""
	cr.TrimLeadingSpace = true

	recs, err := cr.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(recs) == 0 || len(recs[0]) == 0 {
		return nil, nil, fmt.Errorf("%s: CSV has no header row", path)
	}
	return recs[0], recs[1:], nil
}

// humanSize renders a byte count the way a human reads it.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < len("KMGTPE")-1; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
