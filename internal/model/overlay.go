package model

// TagOrigin — происхождение тега с учётом наложения пользователя.
type TagOrigin uint8

const (
	OriginMeta   TagOrigin = iota // оригинальный из meta.json, не скрыт
	OriginCustom                  // свой
	OriginHidden                  // оригинальный, скрытый пользователем
)

// TagView — тег с происхождением.
type TagView struct {
	Tag
	Origin TagOrigin
}

// EffectiveTags возвращает действующие теги: оригинальные без скрытых
// в порядке meta.json, затем свои в порядке добавления.
func (g Gallery) EffectiveTags() []Tag {
	if len(g.Custom) == 0 && len(g.Hidden) == 0 {
		return g.Tags
	}
	out := make([]Tag, 0, len(g.Tags)+len(g.Custom))
	for _, t := range g.Tags {
		if !g.IsHidden(t) {
			out = append(out, t)
		}
	}
	return append(out, g.Custom...)
}

// TagViews возвращает все теги с происхождением: оригинальные (скрытые на
// своём месте) в порядке meta.json, затем свои. Скрытый тег, которого нет
// среди оригинальных, не возвращается.
func (g Gallery) TagViews() []TagView {
	out := make([]TagView, 0, len(g.Tags)+len(g.Custom))
	for _, t := range g.Tags {
		o := OriginMeta
		if g.IsHidden(t) {
			o = OriginHidden
		}
		out = append(out, TagView{Tag: t, Origin: o})
	}
	for _, t := range g.Custom {
		out = append(out, TagView{Tag: t, Origin: OriginCustom})
	}
	return out
}

// IsHidden сообщает, скрыт ли тег пользователем.
func (g Gallery) IsHidden(t Tag) bool {
	for _, h := range g.Hidden {
		if h == t {
			return true
		}
	}
	return false
}

// IsCustom сообщает, есть ли такой свой тег.
func (g Gallery) IsCustom(t Tag) bool {
	for _, c := range g.Custom {
		if c == t {
			return true
		}
	}
	return false
}
