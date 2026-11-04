package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/enowx/enowx/core/designbrief"
	"github.com/enowx/enowx/proto"
)

const briefUsage = `brief CANVAS [--set TEXT|-]      what the canvas's designs are for: product, users, tone, first screen…;
                                   --set writes it, as Markdown a section a field (- reads stdin)`

// canvasBrief prints canvas's brief, or the template to fill when it has
// none; --set writes it.
func (r runner) canvasBrief(args []string) error {
	fs := flag.NewFlagSet("brief", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	set := fs.String("set", "", "")
	pos, err := parseAll(fs, args)
	if err != nil || len(pos) != 1 {
		return errors.New("usage: enowx-app canvas " + briefUsage)
	}
	c, err := r.findCanvas(pos[0])
	if err != nil {
		return err
	}
	if *set == "" {
		defer r.canvasReferences(c)
		if c.Brief == nil {
			fmt.Fprintf(r.out, "No brief yet. Write one: enowx-app canvas brief %s --set -, with these sections:\n\n%s", c.ID, designbrief.Template())
			return nil
		}
		fmt.Fprint(r.out, designbrief.Markdown(*c.Brief))
		return nil
	}
	text, err := stdinOr(*set)
	if err != nil {
		return err
	}
	b := designbrief.Parse(text)
	if err := designbrief.Check(b); err != nil {
		return fmt.Errorf("canvas: %w\n\n%s", err, designbrief.Template())
	}
	if _, err := r.c.EditCanvas(proto.CanvasEdit{ID: c.ID, Op: proto.CanvasOpBrief, Brief: &b, Session: os.Getenv("ENOWX_SESSION")}); err != nil {
		return err
	}
	fmt.Fprintln(r.out, "Brief kept; the app shows it on the canvas.")
	return nil
}

// stdinOr is v, or what stdin holds when v is "-".
func stdinOr(v string) (string, error) {
	if v != "-" {
		return v, nil
	}
	data, err := io.ReadAll(os.Stdin)
	return string(data), err
}
