package audit

// Observer интерфейс наблюдателя (подписчика)
type Observer interface {
	Update(event *Event) error
}

// Subject интерфейс издателя (субъекта)
type Subject interface {
	Register(observer Observer)
	Deregister(observer Observer)
	Notify(event *Event)
}
