package runlua

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	lua "github.com/J-J-J/goluajit"
)

func luaErrorType(apierr *lua.APIError) string {
	switch apierr.Type {
	case lua.APIErrorSyntax:
		return "APIErrorSyntax"
	case lua.APIErrorFile:
		return "APIErrorFile"
	case lua.APIErrorRun:
		return "APIErrorRun"
	case lua.APIErrorError:
		return "APIErrorError"
	case lua.APIErrorPanic:
		return "APIErrorPanic"
	default:
		return "unknown"
	}
}

var reNumber = regexp.MustCompile("\\d+")

func stackTraceWithCode(stacktrace string, code string) string {
	var result []string

	stlines := strings.Split(stacktrace, "\n")
	lines := strings.Split(code, "\n")
	result = append(result, stlines[0])

	for i := 1; i < len(stlines); i++ {
		stline := stlines[i]
		result = append(result, stline)

		snum := reNumber.FindString(stline)
		if snum != "" {
			num, _ := strconv.Atoi(snum)
			for i, line := range lines {
				line = fmt.Sprintf("%3d %s", i+1, line)
				if i+1 > num-3 && i+1 < num+3 {
					result = append(result, line)
				}
			}
		}
	}

	return strings.Join(result, "\n")
}
