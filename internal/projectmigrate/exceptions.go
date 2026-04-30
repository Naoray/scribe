package projectmigrate

var globalSkillExceptions = map[string]struct{}{
	"scribe": {},
}

func isGlobalSkillException(skill string) bool {
	_, ok := globalSkillExceptions[skill]
	return ok
}
