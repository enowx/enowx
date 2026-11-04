package library

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/enowx/enowx/cloud/api"
)

// What a profile may say. It's public, and people click what's on it: a link only
// ever goes to a web address, and an email address stays off it altogether.

var (
	emailShape = regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9-]+(\.[a-z0-9-]+)*\.[a-z]{2,}`)
	zoneShape  = regexp.MustCompile(`^(?:UTC|GMT)?\s*([+-]?)(\d{1,2})(?::?(\d{2}))?$`)
)

// handleProblem says what's wrong with a new handle, "" for nothing.
func handleProblem(h string) string { return api.LibraryHandleProblem(h) }

// profileLink is u when a profile may link to it: a web address, with no name or
// password in it, and no email hiding in it.
func profileLink(u string) (string, error) {
	u = strings.TrimSpace(u)
	if len(u) > 300 {
		return "", errors.New("a link is 300 characters at most")
	}
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "https" && p.Scheme != "http") || p.Hostname() == "" || p.User != nil {
		return "", errors.New("a link starts with https:// or http://")
	}
	if emailShape.MatchString(u) {
		return "", errors.New("take the email address out of the link: enowX UI never shows one")
	}
	return p.String(), nil
}

// timeZoneOf is what someone typed for a time zone as the list keeps it: UTC+7,
// UTC-3:30. Empty stays empty.
func timeZoneOf(s string) (string, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return "", nil
	}
	bad := errors.New("say a time zone as an offset from UTC, like UTC+7 or UTC-3:30")
	m := zoneShape.FindStringSubmatch(s)
	if m == nil {
		return "", bad
	}
	hours, _ := strconv.Atoi(m[2])
	mins := 0
	if m[3] != "" {
		mins, _ = strconv.Atoi(m[3])
	}
	sign := m[1]
	if sign == "" {
		sign = "+"
	}
	if mins != 0 && mins != 30 && mins != 45 {
		return "", bad
	}
	value := float64(hours) + float64(mins)/60
	if (sign == "-" && value > 12) || (sign == "+" && value > 14) {
		return "", bad
	}
	if hours == 0 && mins == 0 {
		return "UTC", nil
	}
	if mins == 0 {
		return "UTC" + sign + strconv.Itoa(hours), nil
	}
	return fmt.Sprintf("UTC%s%d:%02d", sign, hours, mins), nil
}

// zonesOf are the time zones a span of the list's filter holds, as timeZoneOf writes
// them; ok false for a span there's no such.
func zonesOf(id string) (zones []string, ok bool) {
	for _, z := range api.LibraryZones {
		if z.ID != id {
			continue
		}
		for quarter := -12 * 4; quarter <= 14*4; quarter++ {
			hours := float64(quarter) / 4
			if hours < z.From || hours > z.To {
				continue
			}
			sign, abs := "+", quarter
			if quarter < 0 {
				sign, abs = "-", -quarter
			}
			name, err := timeZoneOf("UTC" + sign + strconv.Itoa(abs/4) + fmt.Sprintf(":%02d", (abs%4)*15))
			if err == nil {
				zones = append(zones, name)
			}
		}
		return zones, true
	}
	return nil, false
}

// cleanProfile is p as it can be kept, or what's wrong with it. handle is the profile's
// handle now: only a new one has to pass the handle's rules.
func cleanProfile(p api.LibraryProfileParams, handle string) (api.LibraryProfileParams, error) {
	p.Handle = strings.ToLower(strings.TrimSpace(p.Handle))
	if p.Handle == "" {
		p.Handle = handle
	}
	if p.Handle != handle {
		if why := handleProblem(p.Handle); why != "" {
			return p, errors.New(why)
		}
	}
	p.Bio = cleanText(p.Bio, api.MaxLibraryBio)
	if emailShape.MatchString(p.Bio) {
		return p, errors.New("take the email address out of your bio: enowX UI never shows one. A contact link can lead to it")
	}
	if len(p.Links) > api.MaxLibraryLinks {
		return p, fmt.Errorf("a profile has %d links at most", api.MaxLibraryLinks)
	}
	links := []string{}
	for _, l := range p.Links {
		if strings.TrimSpace(l) == "" {
			continue
		}
		link, err := profileLink(l)
		if err != nil {
			return p, err
		}
		if !slices.Contains(links, link) {
			links = append(links, link)
		}
	}
	p.Links = links
	kinds := []string{}
	for _, k := range p.WorkKinds {
		if !oneOf(api.LibraryWorkKinds, k) {
			return p, errors.New("pick the kinds of work from the list")
		}
		if !slices.Contains(kinds, k) {
			kinds = append(kinds, k)
		}
	}
	p.WorkKinds = kinds
	if len(p.Skills) > api.MaxLibrarySkills {
		return p, fmt.Errorf("list %d skills at most", api.MaxLibrarySkills)
	}
	skills := []string{}
	for _, raw := range p.Skills {
		skill := strings.ToLower(cleanText(raw, 32))
		if skill == "" {
			continue
		}
		if emailShape.MatchString(skill) {
			return p, errors.New("take the email address out of your skills")
		}
		if !slices.Contains(skills, skill) {
			skills = append(skills, skill)
		}
	}
	p.Skills = skills
	var err error
	if p.TimeZone, err = timeZoneOf(p.TimeZone); err != nil {
		return p, err
	}
	if strings.TrimSpace(p.ContactURL) != "" {
		if p.ContactURL, err = profileLink(p.ContactURL); err != nil {
			return p, fmt.Errorf("the contact: %w", err)
		}
	} else {
		p.ContactURL = ""
	}
	if p.OpenForWork && p.ContactURL == "" {
		return p, errors.New("open for work needs a contact link, as the site has no messages")
	}
	return p, nil
}
