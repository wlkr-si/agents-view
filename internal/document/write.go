package document

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Update(file, name, replacement string, expected *string, appendBody bool) error {
	if TrimSpace(name) == "" || name != TrimSpace(name) || strings.ContainsAny(name, "\r\n") || headingTail.MatchString(name) {
		return fmt.Errorf("Use a single section heading.")
	}
	if len(Parse(replacement).Sections) > 1 {
		return fmt.Errorf("A section body cannot include another level-two heading.")
	}

	_, err := updateFile(file, nil, func(text string) (string, error) {
		return updateSection(text, name, replacement, expected, appendBody)
	})
	return err
}

// Revision identifies the exact bytes read from disk, including line endings.
func Revision(text string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
}

var ErrChanged = errors.New("AGENTS.md changed on disk.")

func Save(file, text, revision string) (string, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	doc := Parse(text)
	if !doc.HasVersion {
		return "", fmt.Errorf("Missing first-line %s tag.", Version)
	}
	names := make(map[string]bool, len(doc.Sections))
	for _, section := range doc.Sections {
		name := section.DisplayName()
		if names[name] {
			return "", fmt.Errorf("Duplicate or reserved section name: %s", name)
		}
		names[name] = true
	}
	return updateFile(file, &revision, func(current string) (string, error) {
		if strings.Contains(current, "\r\n") {
			return strings.ReplaceAll(text, "\n", "\r\n"), nil
		}
		return text, nil
	})
}

func AddOverride(file, quote, replacement, revision string) (string, error) {
	return updateFile(file, &revision, func(text string) (string, error) {
		return updateSection(text, "Overrides", "- Global: "+quote+" → here: "+replacement, nil, true)
	})
}

func updateFile(file string, revision *string, update func(string) (string, error)) (string, error) {
	target, err := filepath.EvalSymlinks(file)
	if err != nil {
		return "", FileError("lstat", err)
	}
	lockPath := target + ".agents-view.lock"
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", FileError("open", err)
	}
	defer os.Remove(lockPath)
	if err := lock.Close(); err != nil {
		return "", FileError("close", err)
	}

	raw, err := os.ReadFile(target)
	if err != nil {
		return "", FileError("open", err)
	}
	text := string(raw)
	if revision != nil && Revision(text) != *revision {
		return "", ErrChanged
	}
	next, err := update(text)
	if err != nil || next == text {
		return next, err
	}
	return next, replaceFile(target, next, text)
}

func updateSection(text, name, replacement string, expected *string, appendBody bool) (string, error) {
	document := Parse(text)
	current, err := document.FindSection(name)
	if err != nil {
		return "", err
	}
	if expected != nil && (current == nil || current.Body != *expected) {
		return "", fmt.Errorf("The section changed on disk. Edit it again using the latest version.")
	}
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	start, end := bodyBounds(replacement)
	body := strings.ReplaceAll(replacement[start:end], "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n", eol)

	next := text
	switch {
	case current == nil:
		if text != "" && !strings.HasSuffix(text, eol+eol) {
			if !strings.HasSuffix(text, eol) {
				next += eol
			}
			next += eol
		}
		next += "## " + name + eol
		if body != "" {
			next += eol + body + eol
		}
	case current.Body != "":
		if appendBody {
			body = current.Body + eol + body
		}
		next = text[:current.BodyStart] + body + text[current.BodyEnd:]
	case body != "":
		before, after := text[:current.Start], text[current.Start:]
		if before != "" {
			if !strings.HasSuffix(before, "\n") {
				before += eol
			}
			before += eol
		}
		next = before + body + eol + after
	}
	if next == text {
		return text, nil
	}
	if !sameStructure(document, Parse(next), current == nil, name) {
		return "", fmt.Errorf("The edit changes the document structure. Check headings, comments, and code fences.")
	}
	return next, nil
}

func sameStructure(before, after Document, created bool, name string) bool {
	count := len(before.Sections)
	if created {
		count++
	}
	if before.HasVersion != after.HasVersion || count != len(after.Sections) {
		return false
	}
	for index, section := range before.Sections {
		if section.Name != after.Sections[index].Name {
			return false
		}
	}
	return !created || after.Sections[count-1].Name == name
}

func replaceFile(target, text, expected string) error {
	info, err := os.Stat(target)
	if err != nil {
		return FileError("stat", err)
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	id[6] = id[6]&0x0f | 0x40
	id[8] = id[8]&0x3f | 0x80
	name := fmt.Sprintf(".agents-view-%x-%x-%x-%x-%x.tmp", id[0:4], id[4:6], id[6:8], id[8:10], id[10:16])
	path := filepath.Join(filepath.Dir(target), name)
	temp, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return FileError("open", err)
	}
	defer os.Remove(path)
	_, writeErr := temp.WriteString(text)
	closeErr := temp.Close()
	if writeErr != nil {
		return FileError("write", writeErr)
	}
	if closeErr != nil {
		return FileError("close", closeErr)
	}
	current, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	if string(current) != expected {
		return ErrChanged
	}
	if err := os.Rename(path, target); err != nil {
		return FileError("rename", err)
	}
	return nil
}
