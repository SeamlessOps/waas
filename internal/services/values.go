package services

import (
	"fmt"
	"regexp"
	"strings"
)

// Values says where in the GitOps repository a service's image is written,
// and in what shape. It is set in layers, each over the one before: the
// default, an environment's "values" (metadata.environments[].values), the
// repository's (repositories.<repo>.values), and the entry's own.
//
//	"values": {
//	  "path": "environment/{environment}",
//	  "key": "services.{name}-{domain}",
//	  "image": { "tag": "tag" }
//	}
//
// path is a values file, or a directory holding exactly one file with the
// service under key. key is the service's dotted path in that file. image
// names the fields written, relative to key:
//
//   - tag: the image tag alone; the chart knows the repository. The default.
//   - repository with tag: the image's repository and its tag.
//   - ref: the full image, registry/name:tag, in one field.
//   - digest, beside any of them: the image's digest, sha256:….
//
// path and key take the placeholders {environment}, {name} and {domain}.
type Values struct {
	Path  string       `json:"path,omitempty"`
	Key   string       `json:"key,omitempty"`
	Image *ImageFields `json:"image,omitempty"`
}

// ImageFields are the fields a deployment writes, each a dotted path
// relative to Values.Key; "" is not written.
type ImageFields struct {
	Tag        string `json:"tag,omitempty"`
	Repository string `json:"repository,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Digest     string `json:"digest,omitempty"`
}

// DefaultValues is where WaaS writes when nothing says otherwise: the tag
// alone, at services.<name>-<domain>.tag, in the one file under
// environment/<environment> that holds the service.
var DefaultValues = Values{
	Path:  "environment/{environment}",
	Key:   "services.{name}-{domain}",
	Image: &ImageFields{Tag: "tag"},
}

// Target is a service's Values resolved for one environment: placeholders
// filled in, and each field a yq path from the file's root.
type Target struct {
	// Path is the values file, or the directory to find it in.
	Path string `json:"path"`
	// Key is the service's yq path, ."services"."api-staging".
	Key string `json:"key"`
	// Tag, Repository, Ref and Digest are the yq paths written; "" is not.
	Tag        string `json:"tag,omitempty"`
	Repository string `json:"repository,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Digest     string `json:"digest,omitempty"`
}

var (
	segmentRE     = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	pathRE        = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)
	placeholderRE = regexp.MustCompile(`\{[^{}]*\}`)
	placeholders  = map[string]bool{"{environment}": true, "{name}": true, "{domain}": true}
)

// parseValues reads one layer of "values" at where (for messages): an object
// of path, key and image, each text, image an object of tag, repository, ref
// and digest. nil when v is unset.
func parseValues(where string, v any, add func(string, ...any)) *Values {
	if v == nil {
		return nil
	}
	obj, ok := v.(map[string]any)
	if !ok {
		add("%s must be an object of path, key and image", where)
		return nil
	}
	out := &Values{}
	for k, field := range obj {
		switch k {
		case "path":
			out.Path = valuesText(where+".path", field, add)
			checkTemplate(where+".path", out.Path, checkPath, add)
		case "key":
			out.Key = valuesText(where+".key", field, add)
			checkTemplate(where+".key", out.Key, checkDotted, add)
		case "image":
			out.Image = parseImageFields(where+".image", field, add)
		default:
			add("%s.%s is not a values field (path, key, image)", where, k)
		}
	}
	return out
}

func parseImageFields(where string, v any, add func(string, ...any)) *ImageFields {
	obj, ok := v.(map[string]any)
	if !ok {
		add("%s must be an object of tag, repository, ref and digest", where)
		return nil
	}
	out := &ImageFields{}
	for k, field := range obj {
		s := valuesText(where+"."+k, field, add)
		switch k {
		case "tag":
			out.Tag = s
		case "repository":
			out.Repository = s
		case "ref":
			out.Ref = s
		case "digest":
			out.Digest = s
		default:
			add("%s.%s is not an image field (tag, repository, ref, digest)", where, k)
			continue
		}
		if s != "" {
			if err := checkDotted(s); err != nil {
				add("%s.%s: %v", where, k, err)
			}
		}
	}
	if err := out.check(); err != nil {
		add("%s: %v", where, err)
	}
	return out
}

// check says whether the fields make one image: a tag or a ref, a ref
// alone, and a repository only beside a tag.
func (i *ImageFields) check() error {
	switch {
	case i.Ref != "" && (i.Tag != "" || i.Repository != ""):
		return fmt.Errorf("ref is the whole image: set it without tag or repository")
	case i.Repository != "" && i.Tag == "":
		return fmt.Errorf("repository needs tag beside it")
	case i.Tag == "" && i.Ref == "" && i.Digest == "":
		return fmt.Errorf("set tag, ref or digest")
	}
	return nil
}

func valuesText(where string, v any, add func(string, ...any)) string {
	s, ok := v.(string)
	if !ok {
		add("%s must be text", where)
	}
	return s
}

// checkTemplate checks a path or key as written: only known placeholders,
// and valid once they are filled in.
func checkTemplate(where, s string, check func(string) error, add func(string, ...any)) {
	if s == "" {
		return
	}
	for _, p := range placeholderRE.FindAllString(s, -1) {
		if !placeholders[p] {
			add("%s: %s is not a placeholder ({environment}, {name}, {domain})", where, p)
			return
		}
	}
	if err := check(placeholderRE.ReplaceAllString(s, "x")); err != nil {
		add("%s: %v", where, err)
	}
}

// checkDotted checks a dotted path: segments of letters, digits, _ and -.
func checkDotted(s string) error {
	for _, seg := range strings.Split(s, ".") {
		if !segmentRE.MatchString(seg) {
			return fmt.Errorf("%q is not a dotted path of letters, digits, _ and -", s)
		}
	}
	return nil
}

// checkPath checks a path in the GitOps repository: relative, inside it,
// of letters, digits, _, -, . and /.
func checkPath(s string) error {
	if !pathRE.MatchString(s) || strings.HasPrefix(s, "/") {
		return fmt.Errorf("%q is not a relative path of letters, digits, _, -, . and /", s)
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == ".." {
			return fmt.Errorf("%q leaves the GitOps repository", s)
		}
	}
	return nil
}

// over is v with layer laid on it: layer's path and key when set, and its
// image, whole, when set (so a layer setting ref drops the tag below it).
func (v Values) over(layer *Values) Values {
	if layer == nil {
		return v
	}
	if layer.Path != "" {
		v.Path = layer.Path
	}
	if layer.Key != "" {
		v.Key = layer.Key
	}
	if layer.Image != nil {
		v.Image = layer.Image
	}
	return v
}

// TargetFor is where e's image is written in env: DefaultValues under env's
// values, e's repository's, and e's own. f may be nil (no services file).
func (f *File) TargetFor(env Environment, e Entry) (Target, error) {
	v := DefaultValues.over(env.Values)
	if f != nil {
		v = v.over(f.repositoryValues(e.Repository()))
	}
	v = v.over(parseValues("values", e["values"], func(string, ...any) {}))
	return v.resolve(env, e.Name())
}

// repositoryValues is repo's "values" layer, nil when it has none.
func (f *File) repositoryValues(repo string) *Values {
	obj, _ := f.Repositories[repo].(map[string]any)
	return parseValues("values", obj["values"], func(string, ...any) {})
}

// resolve fills v's placeholders for env and the service name, and turns
// its fields into yq paths.
func (v Values) resolve(env Environment, name string) (Target, error) {
	fill := strings.NewReplacer("{environment}", env.Name, "{name}", name, "{domain}", env.Domain)
	path, key := fill.Replace(v.Path), fill.Replace(v.Key)
	if err := checkPath(path); err != nil {
		return Target{}, fmt.Errorf("service %s in %s: values.path: %w", name, env.Name, err)
	}
	if err := checkDotted(key); err != nil {
		return Target{}, fmt.Errorf("service %s in %s: values.key: %w", name, env.Name, err)
	}
	img := v.Image
	if img == nil {
		img = DefaultValues.Image
	}
	if err := img.check(); err != nil {
		return Target{}, fmt.Errorf("service %s in %s: values.image: %w", name, env.Name, err)
	}
	at := func(field string) string {
		if field == "" {
			return ""
		}
		return yqPath(key + "." + field)
	}
	return Target{
		Path:       path,
		Key:        yqPath(key),
		Tag:        at(img.Tag),
		Repository: at(img.Repository),
		Ref:        at(img.Ref),
		Digest:     at(img.Digest),
	}, nil
}

// yqPath is a checked dotted path as a yq (and jq) path, each segment
// quoted: services.api-staging.tag is ."services"."api-staging"."tag".
func yqPath(dotted string) string {
	var b strings.Builder
	for _, seg := range strings.Split(dotted, ".") {
		b.WriteString(`."`)
		b.WriteString(seg)
		b.WriteString(`"`)
	}
	return b.String()
}
