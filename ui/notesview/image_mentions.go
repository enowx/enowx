package notesview

import (
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

type noteImage struct{ name, ref string }

func (e *editor) noteImages() []noteImage {
	var images []noteImage
	seen := map[string]bool{}
	for _, b := range e.blocks {
		if b.kind == kCode {
			continue
		}
		for _, m := range imageMatches(b.text) {
			ref := imageSource(b.text[m[4]:m[5]])
			if seen[ref] {
				continue
			}
			seen[ref] = true
			name := strings.TrimSpace(b.text[m[2]:m[3]])
			if name == "" || strings.EqualFold(name, "Image") {
				name = "Image " + strconv.Itoa(len(images)+1)
			}
			images = append(images, noteImage{name, ref})
		}
	}
	for _, img := range e.relatedImages {
		if !seen[img.ref] {
			images = append(images, img)
			seen[img.ref] = true
		}
	}
	return images
}

func (e *editor) imageName(ref string) string {
	for _, img := range e.noteImages() {
		if img.ref == ref {
			return img.name
		}
	}
	return path.Base(ref)
}

func imageReference(name, ref string) string {
	name = strings.NewReplacer("[", "", "]", "", "\n", " ", "\r", " ").Replace(name)
	return "[@" + name + "](" + ref + ")"
}

// imageMentionAt recognizes @queries at word boundaries, using rune carets.
func imageMentionAt(text string, caret int) (start, stop int, query string, ok bool) {
	runes := []rune(text)
	if caret < 0 || caret > len(runes) {
		return
	}
	for at := caret - 1; at >= 0; at-- {
		r := runes[at]
		if r == '@' {
			if at > 0 && !unicode.IsSpace(runes[at-1]) && !strings.ContainsRune("([{", runes[at-1]) {
				return
			}
			return at, caret, strings.ToLower(string(runes[at+1 : caret])), true
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' && r != '.' {
			return
		}
	}
	return
}

func (e *editor) imageMentionMatches(text string, caret int) (int, int, []noteImage) {
	start, stop, q, ok := imageMentionAt(text, caret)
	if !ok || text == e.mentionDismissed {
		return 0, 0, nil
	}
	var matches []noteImage
	for _, img := range e.noteImages() {
		if strings.Contains(strings.ToLower(img.name+" "+path.Base(img.ref)), q) {
			matches = append(matches, img)
		}
	}
	return start, stop, matches
}

func insertImageMention(text string, start, stop int, img noteImage) (string, int) {
	runes := []rune(text)
	link := imageReference(img.name, img.ref) + " "
	return string(runes[:start]) + link + string(runes[stop:]), start + utf8.RuneCountInString(link)
}

func (e *editor) imageMentionKey(k ui.TextKey, set func(string, int)) bool {
	if !e.mentionActive || k.Mods != 0 {
		return false
	}
	text := k.Text()
	start, stop, matches := e.imageMentionMatches(text, e.mentionCaret)
	if len(matches) == 0 {
		return false
	}
	switch k.Key {
	case ui.KeyDown:
		e.mentionPick = (e.mentionPick + 1) % len(matches)
	case ui.KeyUp:
		e.mentionPick = (e.mentionPick + len(matches) - 1) % len(matches)
	case ui.KeyEnter, ui.KeyTab:
		value, caret := insertImageMention(text, start, stop, matches[min(e.mentionPick, len(matches)-1)])
		k.SetText(value)
		set(value, caret)
		e.mentionActive = false
	case ui.KeyEscape:
		e.mentionDismissed, e.mentionActive = text, false
	default:
		return false
	}
	return true
}

func (e *editor) imageMentionMenu(c *ui.Context, in ui.Element, text string, set func(string, int)) {
	if !in.Focused() && !e.mentionActive {
		return
	}
	startSel, endSel := in.TextSelection()
	e.mentionCaret = endSel
	start, stop, matches := e.imageMentionMatches(text, endSel)
	e.mentionActive = startSel == endSel && len(matches) > 0
	if !e.mentionActive {
		return
	}
	e.mentionPick = min(e.mentionPick, len(matches)-1)
	ui.Overlay(c, func() {
		menu := widget.Popup(ui.Column(c).AttachTo(in, ui.AnchorBottomStart, ui.AnchorTopStart).Width(264).MaxHeight(widget.PopupRoom(c, in, 280)).Margin(theme.S1, 0, 0, 0))
		e.menuIn = e.menuIn || menu.Hovered()
		menu.Children(func() {
			widget.PopupLabel(c, "Mention an image")
			widget.Scroll(c).FillWidth().Shrink(1).MinHeight(0).Children(func() {
				for j, img := range matches {
					row := ui.ButtonBase(c.Key(img.ref)).Justify(ui.Start).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("Choose image " + img.name)
					widget.PopupItem(row, j == e.mentionPick || row.Hovered()).Children(func() {
						ui.Icon(c, icon.Image).FontSize(14).TextColor(theme.TextSoft)
						ui.Column(c).Grow(1).MinWidth(0).Children(func() {
							widget.Running(c, img.name, widget.RunStyle{})
							widget.Running(c.Key("file"), path.Base(img.ref), widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
						})
					})
					if row.Clicked() {
						value, caret := insertImageMention(text, start, stop, img)
						set(value, caret)
						e.mentionActive = false
					}
				}
			})
		})
	})
}
