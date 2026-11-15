package web

import "html/template"

// Monochrome line icons (24×24 viewBox, stroke = currentColor). Kept as
// plain inline SVG with no filters/masks so old WebKit renders them.
var icons = map[string]string{
	"sun":     `<circle cx="12" cy="12" r="4.5"/><path d="M12 1.5v3M12 19.5v3M1.5 12h3M19.5 12h3M4.6 4.6l2.1 2.1M17.3 17.3l2.1 2.1M4.6 19.4l2.1-2.1M17.3 6.7l2.1-2.1"/>`,
	"partly":  `<path d="M8 2.5v1.8M2.5 8h1.8M4.1 4.1l1.3 1.3M12 4.1l-1.3 1.3"/><path d="M5.4 10.2A3.6 3.6 0 0 1 11.6 6.6"/><path d="M8 20h10a4 4 0 0 0 .6-7.95A5.5 5.5 0 0 0 8.1 12 4 4 0 0 0 8 20z"/>`,
	"cloud":   `<path d="M7 19h11a4.5 4.5 0 0 0 .7-8.95A6 6 0 0 0 7.2 9.1 5 5 0 0 0 7 19z"/>`,
	"fog":     `<path d="M7 13h11a4 4 0 0 0 .5-7.95A5.5 5.5 0 0 0 7.2 4.6 4.3 4.3 0 0 0 7 13z"/><path d="M3 17h18M5 21h14"/>`,
	"drizzle": `<path d="M7 14h11a4 4 0 0 0 .5-7.95A5.5 5.5 0 0 0 7.2 5.6 4.3 4.3 0 0 0 7 14z"/><path d="M8 18v1M12 18v1M16 18v1M10 21.5v.5M14 21.5v.5"/>`,
	"rain":    `<path d="M7 14h11a4 4 0 0 0 .5-7.95A5.5 5.5 0 0 0 7.2 5.6 4.3 4.3 0 0 0 7 14z"/><path d="M8 17l-1.5 4M12.5 17L11 21M17 17l-1.5 4"/>`,
	"storm":   `<path d="M7 14h11a4 4 0 0 0 .5-7.95A5.5 5.5 0 0 0 7.2 5.6 4.3 4.3 0 0 0 7 14z"/><path d="M12.5 15l-3 4h4l-2.5 4"/>`,
	"snow":    `<path d="M7 14h11a4 4 0 0 0 .5-7.95A5.5 5.5 0 0 0 7.2 5.6 4.3 4.3 0 0 0 7 14z"/><path d="M8 18.5h.01M12 18.5h.01M16 18.5h.01M10 21.5h.01M14 21.5h.01"/>`,
	"drop":    `<path d="M12 2.5s6.5 7.2 6.5 11.5a6.5 6.5 0 0 1-13 0C5.5 9.7 12 2.5 12 2.5z"/>`,
	"wind":    `<path d="M3 9h11.5a3 3 0 1 0-3-3M3 15h15.5a3 3 0 1 1-3 3M3 12h8"/>`,
	"gold":    `<path d="M3 20h18l-2.5-6h-13zM7 14l2-5h6l2 5"/><path d="M10 9l1-3h2l1 3"/>`,
	"trend":   `<path d="M3 17l6-6 4 4 8-8"/><path d="M15 7h6v6"/>`,
	"claude":  `<path d="M12 2.5v19M2.5 12h19M5.3 5.3l13.4 13.4M18.7 5.3L5.3 18.7"/>`,
	"codex":   `<rect x="2.5" y="4" width="19" height="16" rx="2.5"/><path d="M7 9.5l3 2.5-3 2.5M12.5 15h4.5"/>`,
	"clock":   `<circle cx="12" cy="12" r="9.5"/><path d="M12 6.5V12l3.5 2"/>`,
	"warn":    `<path d="M12 3L1.8 20.5h20.4z"/><path d="M12 9.5v5M12 17.5v.5"/>`,
	"sync":    `<path d="M20 11a8 8 0 0 0-14.3-4.9L3.5 8.5M4 13a8 8 0 0 0 14.3 4.9l2.2-2.4"/><path d="M3.5 3.5v5h5M20.5 20.5v-5h-5"/>`,
}

// Icon renders an inline SVG icon of the given pixel size.
func Icon(name string, size int) template.HTML {
	p, ok := icons[name]
	if !ok {
		p = icons["cloud"]
	}
	s := itoa(size)
	return template.HTML(`<svg class="ic" width="` + s + `" height="` + s + `" viewBox="0 0 24 24" fill="none" stroke="#000" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">` + p + `</svg>`)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		b[n] = '-'
	}
	return string(b[n:])
}
