package creds

import (
	"fmt"
	"strings"
)

const (
	awsManagedStart = "# BEGIN QUIKSTRATE MANAGED VALUES"
	awsManagedEnd   = "# END QUIKSTRATE MANAGED VALUES"
)

type awsConfigSection struct {
	header     string
	headerLine string
	lines      []string
}

type awsManagedSection struct {
	header string
	values [][2]string
}

// updateManagedAWSConfig replaces values owned by quikstrate while retaining
// all other sections, comments, and values in the AWS config. When prune is
// true, managed values from profiles that are no longer requested are removed.
func updateManagedAWSConfig(content string, managed []awsManagedSection, prune bool) string {
	sections := parseAWSConfig(content)
	wanted := make(map[string]awsManagedSection, len(managed))
	for _, section := range managed {
		wanted[section.header] = section
	}

	for i := range sections {
		if _, isWanted := wanted[sections[i].header]; prune || isWanted {
			sections[i].lines, _ = removeManagedAWSValues(sections[i].lines)
		}
		if prune {
			sections[i].lines = removeLegacyQuikstrateValues(sections[i])
		}
	}

	for _, spec := range managed {
		index := findAWSSection(sections, spec.header)
		if index == -1 {
			sections = append(sections, awsConfigSection{header: spec.header})
			index = len(sections) - 1
		}

		keys := make(map[string]bool, len(spec.values))
		for _, value := range spec.values {
			keys[value[0]] = true
		}
		sections[index].lines = removeAWSKeys(sections[index].lines, keys)
		sections[index].lines = append(sections[index].lines, awsManagedStart)
		for _, value := range spec.values {
			sections[index].lines = append(sections[index].lines, fmt.Sprintf("%s = %s", value[0], value[1]))
		}
		sections[index].lines = append(sections[index].lines, awsManagedEnd)
	}

	sections = removeEmptyAWSSections(sections)
	return renderAWSConfig(sections)
}

func parseAWSConfig(content string) []awsConfigSection {
	sections := []awsConfigSection{{}}
	for _, line := range strings.Split(strings.TrimSuffix(content, "\n"), "\n") {
		if header, ok := parseAWSSectionHeader(line); ok {
			sections = append(sections, awsConfigSection{header: header, headerLine: line})
			continue
		}
		sections[len(sections)-1].lines = append(sections[len(sections)-1].lines, line)
	}
	return sections
}

// parseAWSSectionHeader returns the canonical section name while permitting
// AWS-supported inline comments after the closing bracket. Keeping the
// canonical name separate from the original line lets us match a managed
// profile without discarding its comment.
func parseAWSSectionHeader(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	closingBracket := strings.IndexByte(trimmed, ']')
	if closingBracket == -1 {
		return "", false
	}
	remainder := strings.TrimSpace(trimmed[closingBracket+1:])
	if remainder != "" && !strings.HasPrefix(remainder, ";") && !strings.HasPrefix(remainder, "#") {
		return "", false
	}
	return trimmed[:closingBracket+1], true
}

func removeManagedAWSValues(lines []string) ([]string, bool) {
	var out []string
	found := false
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != awsManagedStart {
			out = append(out, lines[i])
			continue
		}
		end := i + 1
		for end < len(lines) && strings.TrimSpace(lines[end]) != awsManagedEnd {
			end++
		}
		if end == len(lines) {
			// A damaged marker must not cause the remainder of a user's section
			// to be discarded. Drop only our unmatched marker and retain the
			// values below it for normal key-level reconciliation.
			out = append(out, lines[i+1:]...)
			found = true
			break
		}
		found = true
		i = end
	}
	return out, found
}

func removeLegacyQuikstrateValues(section awsConfigSection) []string {
	keys := map[string]bool{}
	if isQuikstrateSSOSection(section.header) {
		keys["sso_start_url"] = true
		keys["sso_region"] = true
		keys["sso_registration_scopes"] = true
	}
	for _, line := range section.lines {
		key, value, ok := awsConfigValue(line)
		if ok && key == "credential_process" && strings.Contains(value, "quikstrate") {
			keys["credential_process"] = true
			keys["region"] = true
		}
	}
	return removeAWSKeys(section.lines, keys)
}

func removeAWSKeys(lines []string, keys map[string]bool) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		key, _, ok := awsConfigValue(line)
		if ok && keys[key] {
			continue
		}
		out = append(out, line)
	}
	return trimBlankAWSLines(out)
}

func awsConfigValue(line string) (string, string, bool) {
	key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(key), strings.TrimSpace(value), true
}

func findAWSSection(sections []awsConfigSection, header string) int {
	for i := range sections {
		if sections[i].header == header {
			return i
		}
	}
	return -1
}

func isQuikstrateSSOSection(header string) bool {
	for _, instance := range []idcInstance{metronomeIDC, stripeIDC, stripeAlternateIDC} {
		if header == fmt.Sprintf("[sso-session %s]", instance.Name) {
			return true
		}
	}
	return false
}

func removeEmptyAWSSections(sections []awsConfigSection) []awsConfigSection {
	out := make([]awsConfigSection, 0, len(sections))
	for _, section := range sections {
		if section.header != "" && len(trimBlankAWSLines(section.lines)) == 0 {
			continue
		}
		out = append(out, section)
	}
	return out
}

func trimBlankAWSLines(lines []string) []string {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}

func renderAWSConfig(sections []awsConfigSection) string {
	var blocks []string
	for _, section := range sections {
		var block []string
		if section.header != "" {
			headerLine := section.headerLine
			if headerLine == "" {
				headerLine = section.header
			}
			block = append(block, headerLine)
		}
		block = append(block, trimBlankAWSLines(section.lines)...)
		if len(block) > 0 {
			blocks = append(blocks, strings.Join(block, "\n"))
		}
	}
	if len(blocks) == 0 {
		return ""
	}
	return strings.Join(blocks, "\n\n") + "\n"
}
