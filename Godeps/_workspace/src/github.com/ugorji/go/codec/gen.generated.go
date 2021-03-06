// Copyright (c) 2012-2015 Ugorji Nwoke. All rights reserved.
// Use of this source code is governed by a BSD-style license found in the LICENSE file.

package codec

// DO NOT EDIT. THIS FILE IS AUTO-GENERATED FROM gen-dec-(map|array).go.tmpl

const genDecMapTmpl = `
{{var "v"}} := *{{ .Varname }}
{{var "l"}} := r.ReadMapStart()
if {{var "v"}} == nil {
	if {{var "l"}} > 0 {
		{{var "v"}} = make(map[{{ .KTyp }}]{{ .Typ }}, {{var "l"}})
	} else {
		{{var "v"}} = make(map[{{ .KTyp }}]{{ .Typ }}) // supports indefinite-length, etc
	}
	*{{ .Varname }} = {{var "v"}}
}
if {{var "l"}} > 0  {
for {{var "j"}} := 0; {{var "j"}} < {{var "l"}}; {{var "j"}}++ {
	var {{var "mk"}} {{ .KTyp }} 
	{{ $x := printf "%vmk%v" .TempVar .Rand }}{{ decLineVarK $x }}
{{ if eq .KTyp "interface{}" }}// special case if a byte array.
	if {{var "bv"}}, {{var "bok"}} := {{var "mk"}}.([]byte); {{var "bok"}} {
		{{var "mk"}} = string({{var "bv"}})
	}
{{ end }}
	{{var "mv"}} := {{var "v"}}[{{var "mk"}}]
	{{ $x := printf "%vmv%v" .TempVar .Rand }}{{ decLineVar $x }}
	if {{var "v"}} != nil {
		{{var "v"}}[{{var "mk"}}] = {{var "mv"}}
	}
}
} else if {{var "l"}} < 0  {
for {{var "j"}} := 0; !r.CheckBreak(); {{var "j"}}++ {
	if {{var "j"}} > 0 {
		r.ReadMapEntrySeparator()
	}
	var {{var "mk"}} {{ .KTyp }} 
	{{ $x := printf "%vmk%v" .TempVar .Rand }}{{ decLineVarK $x }}
{{ if eq .KTyp "interface{}" }}// special case if a byte array.
	if {{var "bv"}}, {{var "bok"}} := {{var "mk"}}.([]byte); {{var "bok"}} {
		{{var "mk"}} = string({{var "bv"}})
	}
{{ end }}
	r.ReadMapKVSeparator()
	{{var "mv"}} := {{var "v"}}[{{var "mk"}}]
	{{ $x := printf "%vmv%v" .TempVar .Rand }}{{ decLineVar $x }}
	if {{var "v"}} != nil {
		{{var "v"}}[{{var "mk"}}] = {{var "mv"}}
	}
}
r.ReadMapEnd()
} // else len==0: TODO: Should we clear map entries?
`

const genDecListTmpl = `
const {{var "Arr"}} bool = {{ .Array }}
{{var "v"}} := *{{ .Varname }}
{{var "h"}}, {{var "l"}} := z.DecSliceHelperStart()

var {{var "c"}} bool 
{{ if not .Array }}if {{var "v"}} == nil {
	if {{var "l"}} <= 0 {
		{{var "v"}} = []{{ .Typ }}{}
	} else {
		{{var "v"}} = make([]{{ .Typ }}, {{var "l"}}, {{var "l"}})
	}
	{{var "c"}} = true 
} 
{{ end }}
if {{var "l"}} == 0 { {{ if not .Array }}
	if len({{var "v"}}) != 0 { 
		{{var "v"}} = {{var "v"}}[:0] 
		{{var "c"}} = true 
	} {{ end }}
} else if {{var "l"}} > 0 {
	{{var "n"}} := {{var "l"}} 
	if {{var "l"}} > cap({{var "v"}}) {
		{{ if .Array }}r.ReadArrayCannotExpand(len({{var "v"}}), {{var "l"}})
		{{var "n"}} = len({{var "v"}})
		{{ else }}{{ if .Immutable }}
		{{var "v2"}} := {{var "v"}}
		{{var "v"}} = make([]{{ .Typ }}, {{var "l"}}, {{var "l"}})
		if len({{var "v"}}) > 0 {
			copy({{var "v"}}, {{var "v2"}}[:cap({{var "v2"}})])
		}
		{{ else }}{{var "v"}} = make([]{{ .Typ }}, {{var "l"}}, {{var "l"}})
		{{ end }}{{var "c"}} = true 
		{{ end }}
	} else if {{var "l"}} != len({{var "v"}}) {
		{{var "v"}} = {{var "v"}}[:{{var "l"}}]
		{{var "c"}} = true 
	}
	{{var "j"}} := 0
	for ; {{var "j"}} < {{var "n"}} ; {{var "j"}}++ {
		{{ $x := printf "%[1]vv%[2]v[%[1]vj%[2]v]" .TempVar .Rand }}{{ decLineVar $x }}
	} {{ if .Array }}
	for ; {{var "j"}} < {{var "l"}} ; {{var "j"}}++ {
		z.DecSwallow()
	}{{ end }}
} else {
	for {{var "j"}} := 0; !r.CheckBreak(); {{var "j"}}++ {
		if {{var "j"}} >= len({{var "v"}}) {
			{{ if .Array }}r.ReadArrayCannotExpand(len({{var "v"}}), {{var "j"}}+1)
			{{ else }}{{var "v"}} = append({{var "v"}}, {{zero}})// var {{var "z"}} {{ .Typ }}
			{{var "c"}} = true {{ end }}
		}
		if {{var "j"}} > 0 {
			{{var "h"}}.Sep({{var "j"}})
		}
		if {{var "j"}} < len({{var "v"}}) {
			{{/* .TempVar }}t{{ .Rand }} := &{{var "v"}}[{{var "j"}}]{{ decLine "t" }} 
			*/}}{{ $x := printf "%[1]vv%[2]v[%[1]vj%[2]v]" .TempVar .Rand }}{{ decLineVar $x }}
		} else {
			{{/* 
			var {{var "z"}} {{ .Typ }}
				{{var "t"}} := &{{var "z"}}{{ decLine "t" }}
			{{ $x := printf "%vz%v" .TempVar .Rand }}{{ decLineVar $x }} 
			*/}}z.DecSwallow()
		}
	}
	{{var "h"}}.End()
}
if {{var "c"}} { 
	*{{ .Varname }} = {{var "v"}}
}
`

