// Package site holds the published page's source assets and embeds them into
// the binary, so a release of fisc carries the site it generates and the
// deploy path needs neither npm nor a checkout of this directory.
//
// The embed lives here rather than in internal/export because //go:embed
// patterns cannot escape the directory of the file that declares them: the
// vendored d3 bundles and .nojekyll are committed under site/, so the
// directive has to be a file under site/ too. internal/export owns everything
// else about packaging; this file owns only the fs.FS.
//
// The patterns are explicit rather than `all:.` on purpose. site/data/ is
// generated output (gitignored, present on a developer's machine, absent in
// CI) and embedding a directory that exists in one place and not the other is
// how a build becomes unreproducible.
package site

import (
	"embed"
	"io/fs"
)

// Assets is the site source tree: the page template, the client script, the
// stylesheet, the vendored d3 bundles with their licences, and the zero-byte
// .nojekyll that stops GitHub Pages running the output through Jekyll.
//
// .nojekyll is named explicitly because //go:embed skips names beginning with
// a dot unless they are matched literally or the pattern carries the `all:`
// prefix — a silently missing sentinel would only show up as a 404 on the
// published site.
//
//go:embed index.html.tmpl trends.html.tmpl chart.html.tmpl provenance.html.tmpl caveats.html.tmpl app.js style.css
//go:embed all:vendor
//go:embed .nojekyll
var Assets embed.FS

// FS returns the embedded assets. Callers take an fs.FS rather than the
// embed.FS so a test can substitute a fstest.MapFS.
func FS() fs.FS { return Assets }
