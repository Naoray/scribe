package projectmigrate

var globalSkillExceptions = map[string]struct{}{
	"scribe-agent": {},
}

func isGlobalSkillException(skill string) bool {
	_, ok := globalSkillExceptions[skill]
	return ok
}
