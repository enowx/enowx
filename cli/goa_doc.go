package cli

import (
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/enowx/enowx/docs"
)

const goaDocUsage = "usage: enowx-app goa doc [NAME] [--section HEADING] | --search WORDS"

// goaDoc prints enowX's docs, which ship in the program: their list with a line
// on each, one of them, one of its sections, or where words are. It needs no
// daemon.
func (r runner) goaDoc(args []string) error {
	fs := flagSet("goa doc")
	section := fs.String("section", "", "")
	search := fs.String("search", "", "")
	words, err := parseAll(fs, args)
	if err != nil || len(words) > 1 || (len(words) == 0 && *section != "") || (*search != "" && (len(words) > 0 || *section != "")) {
		return errors.New(goaDocUsage)
	}
	if *search != "" {
		hits := docs.Search(*search)
		if len(hits) == 0 {
			fmt.Fprintln(r.out, "No doc holds that.")
		}
		for _, h := range hits {
			fmt.Fprintf(r.out, "%s > %s: %s\n", h.Doc, h.Section, h.Line)
		}
		return nil
	}
	if len(words) == 0 {
		return r.goaDocList()
	}
	text, err := docs.Text(words[0])
	if err != nil {
		return err
	}
	if *section != "" {
		if text, err = docs.Section(text, *section); err != nil {
			return err
		}
	}
	fmt.Fprint(r.out, text)
	return nil
}

func (r runner) goaDocList() error {
	w := tabwriter.NewWriter(r.out, 0, 2, 2, ' ', 0)
	for _, name := range docs.Names() {
		text, err := docs.Text(name)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s\t%s\n", name, docs.Summary(text))
	}
	return w.Flush()
}
