package webfetch

import "strings"

// preapprovedRaw is the list of preapproved hosts from Claude Code's preapproved.ts.
// Entries may be hostname-only ("go.dev") or path-scoped ("github.com/anthropics").
// GET requests to these hosts bypass the permission prompt.
var preapprovedRaw = []string{
	// Anthropic
	"platform.claude.com",
	"code.claude.com",
	"modelcontextprotocol.io",
	"github.com/anthropics",
	"agentskills.io",

	// Top Programming Languages
	"docs.python.org",
	"en.cppreference.com",
	"docs.oracle.com",
	"learn.microsoft.com",
	"developer.mozilla.org",
	"go.dev",
	"pkg.go.dev",
	"www.php.net",
	"docs.swift.org",
	"kotlinlang.org",
	"ruby-doc.org",
	"doc.rust-lang.org",
	"www.typescriptlang.org",

	// Web & JavaScript Frameworks/Libraries
	"react.dev",
	"angular.io",
	"vuejs.org",
	"nextjs.org",
	"expressjs.com",
	"nodejs.org",
	"bun.sh",
	"jquery.com",
	"getbootstrap.com",
	"tailwindcss.com",
	"d3js.org",
	"threejs.org",
	"redux.js.org",
	"webpack.js.org",
	"jestjs.io",
	"reactrouter.com",

	// Python Frameworks & Libraries
	"docs.djangoproject.com",
	"flask.palletsprojects.com",
	"fastapi.tiangolo.com",
	"pandas.pydata.org",
	"numpy.org",
	"www.tensorflow.org",
	"pytorch.org",
	"scikit-learn.org",
	"matplotlib.org",
	"requests.readthedocs.io",
	"jupyter.org",

	// PHP Frameworks
	"laravel.com",
	"symfony.com",
	"wordpress.org",

	// Java Frameworks & Libraries
	"docs.spring.io",
	"hibernate.org",
	"tomcat.apache.org",
	"gradle.org",
	"maven.apache.org",

	// .NET & C# Frameworks
	"asp.net",
	"dotnet.microsoft.com",
	"nuget.org",
	"blazor.net",

	// Mobile Development
	"reactnative.dev",
	"docs.flutter.dev",
	"developer.apple.com",
	"developer.android.com",

	// Data Science & Machine Learning
	"keras.io",
	"spark.apache.org",
	"huggingface.co",
	"www.kaggle.com",

	// Databases
	"www.mongodb.com",
	"redis.io",
	"www.postgresql.org",
	"dev.mysql.com",
	"www.sqlite.org",
	"graphql.org",
	"prisma.io",

	// Cloud & DevOps
	"docs.aws.amazon.com",
	"cloud.google.com",
	"kubernetes.io",
	"www.docker.com",
	"docs.docker.com",
	"www.terraform.io",
	"www.ansible.com",
	"vercel.com/docs",
	"docs.netlify.com",
	"devcenter.heroku.com",

	// Testing & Monitoring
	"cypress.io",
	"selenium.dev",

	// Game Development
	"docs.unity.com",
	"docs.unrealengine.com",

	// Other Essential Tools
	"git-scm.com",
	"nginx.org",
	"httpd.apache.org",
}

// hostnameOnly maps plain hostnames → true for O(1) lookup.
var hostnameOnly = make(map[string]bool)

// pathPrefixes maps hostname → list of path prefixes for path-scoped entries.
var pathPrefixes = make(map[string][]string)

func init() {
	for _, entry := range preapprovedRaw {
		slash := strings.IndexByte(entry, '/')
		if slash == -1 {
			hostnameOnly[entry] = true
		} else {
			host := entry[:slash]
			path := entry[slash:]
			pathPrefixes[host] = append(pathPrefixes[host], path)
		}
	}
}

// isPreapprovedHost reports whether the given hostname+pathname combination
// is in the preapproved list. Path-scoped entries enforce segment boundaries
// so "github.com/anthropics" does not match "github.com/anthropics-evil".
func isPreapprovedHost(hostname, pathname string) bool {
	if hostnameOnly[hostname] {
		return true
	}
	prefixes, ok := pathPrefixes[hostname]
	if !ok {
		return false
	}
	for _, p := range prefixes {
		if pathname == p || strings.HasPrefix(pathname, p+"/") {
			return true
		}
	}
	return false
}
