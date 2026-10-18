package text

func (v *Viewer) RevealCursor() { v.mouseScrolled = false }

// Scroll keeps the selection and search position while moving display rows.
func (v *Viewer) Scroll(rows int) {
	total := len(v.Lines)
	if v.Wrap && v.width > 0 {
		starts := v.wrapStarts(v.width)
		total = starts[len(v.Lines)]
	}
	v.Top = max(0, min(v.Top+rows, max(0, total-v.height)))
	v.mouseScrolled = true
}
