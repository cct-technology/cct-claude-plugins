package hook

import (
	"fmt"
	"slices"
	"testing"
)

func installed(line string, powershell bool) []string {
	out := []string{}
	for _, p := range installs(line, powershell) {
		out = append(out, fmt.Sprintf("%s %s %s", p.Ecosystem, p.Name, p.Version))
	}
	return out
}

func TestInstallsFindsThePackagesAShellLineInstalls(t *testing.T) {
	cases := []struct {
		line string
		want []string
	}{
		// npm and its kin
		{"npm install left-pad", []string{"npm left-pad "}},
		{"npm i left-pad@1.3.0 @types/node@^22 --save-dev", []string{"npm left-pad 1.3.0", "npm @types/node ^22"}},
		{"npm add -D typescript", []string{"npm typescript "}},
		{"npm install --prefix web --registry https://registry.npmjs.org react", []string{"npm react "}},
		{"npm install -g pnpm@9", []string{"npm pnpm 9"}},
		{"pnpm add zod", []string{"npm zod "}},
		{"yarn add react react-dom", []string{"npm react ", "npm react-dom "}},
		{"bun add hono@4", []string{"npm hono 4"}},
		{"npm install alias@npm:real-name@2.0.0", []string{"npm real-name 2.0.0"}},
		// not from the registry, or no package at all
		{"npm install", nil},
		{"npm ci", nil},
		{"npm run build", nil},
		{"npm install ./local ../other /abs/path ~/home file:../x git+https://example.com/x.git user/repo https://example.com/x.tgz x.tgz", nil},
		{"yarn install", nil},
		// go
		{"go get github.com/spf13/cobra@v1.8.0", []string{"go github.com/spf13/cobra v1.8.0"}},
		{"go get -u golang.org/x/text", []string{"go golang.org/x/text "}},
		{"go install golang.org/x/tools/cmd/goimports@latest", []string{"go golang.org/x/tools/cmd/goimports latest"}},
		{"go get ./... && go get . && go install ./cmd/qk", nil},
		{"go mod tidy", nil},
		// composer
		{"composer require monolog/monolog", []string{"composer monolog/monolog "}},
		{"composer require --dev phpunit/phpunit:^11 symfony/mailer ^7.1", []string{"composer phpunit/phpunit ^11", "composer symfony/mailer ^7.1"}},
		{"composer require php:^8.3 ext-json", nil},
		// cargo
		{"cargo add serde", []string{"cargo serde "}},
		{"cargo add tokio@1.38 --features full -F rt", []string{"cargo tokio 1.38"}},
		{"cargo install ripgrep", []string{"cargo ripgrep "}},
		{"cargo add local --path ../local", nil},
		{"cargo add forked --git https://example.com/forked", nil},
		// python
		{"pip install requests", []string{"pypi requests "}},
		{`pip install "requests>=2.31" 'uvicorn[standard]==0.30.0' Flask`, []string{"pypi requests >=2.31", "pypi uvicorn ==0.30.0", "pypi flask "}},
		{"pip3 install --upgrade pip", []string{"pypi pip "}},
		{"python -m pip install httpx", []string{"pypi httpx "}},
		{"python3 -m pip install -r requirements.txt -e . -c constraints.txt", nil},
		{"uv add pydantic", []string{"pypi pydantic "}},
		{"uv pip install ruff", []string{"pypi ruff "}},
		{"poetry add django@^5.0 --group dev pytest", []string{"pypi django ^5.0", "pypi pytest "}},
		// around the command
		{"cd web && npm install left-pad", []string{"npm left-pad "}},
		{"sudo npm install -g left-pad", []string{"npm left-pad "}},
		{"NODE_ENV=production npm install left-pad", []string{"npm left-pad "}},
		{"env CI=1 npm install left-pad; pip install requests", []string{"npm left-pad ", "pypi requests "}},
		{"if [ -f package.json ]; then npm install left-pad; fi", []string{"npm left-pad "}},
		{"(cd web; npm install left-pad) | tee log", []string{"npm left-pad "}},
		{"./node_modules/.bin/npm install left-pad", []string{"npm left-pad "}},
		// a line that only mentions an install
		{`echo "npm install left-pad"`, nil},
		{`grep -r "pip install" README.md`, nil},
		{`git commit -m "docs: say npm install left-pad"`, nil},
		{"cat <<'EOF'\nnpm install left-pad\nEOF", nil},
		// a name that is not written on the line cannot be checked
		{`npm install "$PKG" left-pad`, []string{"npm left-pad "}},
		{"npm install $(cat list.txt)", nil},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			if got := installed(c.line, false); !slices.Equal(got, append([]string{}, c.want...)) {
				t.Errorf("installs(%q)\n got %q\nwant %q", c.line, got, c.want)
			}
		})
	}
}

func TestInstallsReadsPowerShell(t *testing.T) {
	cases := []struct {
		line string
		want []string
	}{
		{"npm install left-pad", []string{"npm left-pad "}},
		{"npm.cmd install left-pad@1.3.0", []string{"npm left-pad 1.3.0"}},
		{"& npm.cmd install left-pad", []string{"npm left-pad "}},
		{`& "C:\Program Files\nodejs\npm.cmd" install left-pad`, []string{"npm left-pad "}},
		{"cd web; npm install left-pad; pip install requests", []string{"npm left-pad ", "pypi requests "}},
		{"Set-Location web\r\nnpm install left-pad", []string{"npm left-pad "}},
		{`$env:NODE_ENV = "production"; npm install left-pad`, []string{"npm left-pad "}},
		{"if (Test-Path package.json) { npm install left-pad }", []string{"npm left-pad "}},
		{"py -m pip install 'requests>=2.31'", []string{"pypi requests >=2.31"}},
		{"go.exe get github.com/spf13/cobra@v1.8.0", []string{"go github.com/spf13/cobra v1.8.0"}},
		{"npm install $pkg left-pad", []string{"npm left-pad "}},
		{`Write-Host "run npm install left-pad; then pip install requests"`, nil},
		{"Get-Content README.md | Select-String 'npm install'", nil},
		{"npm run build", nil},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			if got := installed(c.line, true); !slices.Equal(got, append([]string{}, c.want...)) {
				t.Errorf("installs(%q)\n got %q\nwant %q", c.line, got, c.want)
			}
		})
	}
}
