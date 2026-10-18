package plousbench

import (
	"errors"
	"fmt"
	"io"
)

func Toolkit(args []string, out io.Writer) error {
	if len(args) != 2 || args[0] != "report" {
		return errors.New("usage: plousbench report REPORT.json")
	}
	r, err := ReadReport(args[1])
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(out, RenderReport(r))
	return err
}
