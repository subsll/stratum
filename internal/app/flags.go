package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"stratum/internal/model"
)

// stringList is a repeatable string flag.
type stringList struct {
	values []string
}

func (s *stringList) String() string { return strings.Join(s.values, ",") }

func (s *stringList) Set(v string) error {
	s.values = append(s.values, v)
	return nil
}

// parseInterspersed parses flags anywhere among positionals. It survives paths
// that start with a dash by requiring "--" before them, and accepts
// --name=value for all flags.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			pos = append(pos, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		value := ""
		hasValue := false
		if idx := strings.IndexByte(name, '='); idx >= 0 {
			value = name[idx+1:]
			name = name[:idx]
			hasValue = true
		}
		if name == "help" || name == "h" {
			return nil, flag.ErrHelp
		}
		f := fs.Lookup(name)
		if f == nil {
			return nil, fmt.Errorf("unknown flag -%s", name)
		}
		if hasValue {
			if err := fs.Set(name, value); err != nil {
				return nil, err
			}
			continue
		}
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			if err := fs.Set(name, "true"); err != nil {
				return nil, err
			}
			continue
		}
		if i+1 >= len(args) {
			return nil, fmt.Errorf("flag -%s needs a value", name)
		}
		i++
		if err := fs.Set(name, args[i]); err != nil {
			return nil, err
		}
	}
	return pos, nil
}

// personFlags provides the common actor flags. Defaults come from the
// environment, then from the case examiner when a workspace is available.
type personFlags struct {
	Name  string
	Org   string
	Email string
}

func (a *App) addPersonFlags(fs *flag.FlagSet, includeBy bool) *personFlags {
	p := &personFlags{
		Name:  strings.TrimSpace(getenv("STRATUM_EXAMINER")),
		Org:   strings.TrimSpace(getenv("STRATUM_ORG")),
		Email: strings.TrimSpace(getenv("STRATUM_EMAIL")),
	}
	fs.StringVar(&p.Name, "examiner", p.Name, "person recording this action")
	if includeBy {
		fs.StringVar(&p.Name, "by", p.Name, "alias for --examiner")
	}
	fs.StringVar(&p.Org, "org", p.Org, "organisation of the person recording this action")
	fs.StringVar(&p.Email, "email", p.Email, "email of the person recording this action")
	return p
}

// Person resolves the actor, inheriting missing fields from fallback.
func (p *personFlags) Person(fallback model.Person, required bool) (model.Person, error) {
	name := strings.TrimSpace(p.Name)
	org := strings.TrimSpace(p.Org)
	email := strings.TrimSpace(p.Email)
	if name == "" && !fallback.Empty() {
		name = fallback.Name
		if org == "" {
			org = fallback.Org
		}
		if email == "" {
			email = fallback.Email
		}
	}
	name, email = splitEmail(name, email)
	if required && name == "" {
		return model.Person{}, errors.New("a person name is required (use --examiner or set STRATUM_EXAMINER)")
	}
	return model.Person{Name: name, Org: org, Email: email}, nil
}

var emailInside = regexp.MustCompile(`^(.*?)\s*<([^<>]+)>\s*$`)

func splitEmail(name, email string) (string, string) {
	if strings.TrimSpace(email) != "" {
		return strings.TrimSpace(name), strings.TrimSpace(email)
	}
	m := emailInside.FindStringSubmatch(strings.TrimSpace(name))
	if len(m) == 3 {
		return strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
	}
	return strings.TrimSpace(name), ""
}

func getenv(k string) string { return strings.TrimSpace(os.Getenv(k)) }
