package progress

type Phase string

const (
	ArtifactBuilding  Phase = "Building artifact"
	ArtifactUploading Phase = "Uploading artifact"
)

type Progress struct {
	Phase      Phase
	ModelID    string
	Bytes      int64
	TotalBytes int64
	Percent    int
	Done       bool
}

type Reporter func(Progress)

type Tracker struct {
	phase    Phase
	modelID  string
	total    int64
	done     int64
	finished bool
	reporter Reporter
}

func NewTracker(phase Phase, modelID string, total int64, reporter Reporter) *Tracker {
	return &Tracker{
		phase:    phase,
		modelID:  modelID,
		total:    total,
		reporter: reporter,
	}
}

func (t *Tracker) Start() {
	t.report(false)
}

func (t *Tracker) NewPhase(phase Phase, total int64) {
	t.phase = phase
	t.done = 0
	t.total = total

	t.report(false)
}

func (t *Tracker) Advance(progress int64) {
	if progress <= 0 || t.finished {
		return
	}

	t.done += progress
	if t.total > 0 && t.done > t.total {
		t.done = t.total
	}
	t.report(false)
}

func (t *Tracker) Finish() {
	t.done = t.total
	t.finished = true
	t.report(true)
}

func (t *Tracker) report(done bool) {
	t.reporter(Progress{
		Phase:      t.phase,
		ModelID:    t.modelID,
		Bytes:      t.done,
		TotalBytes: t.total,
		Percent:    Percent(t.done, t.total),
		Done:       done,
	})
}

func Percent(done, total int64) int {
	if total <= 0 {
		return 0
	}

	percent := done * 100 / total
	if percent > 100 {
		return 100
	}

	return int(percent)
}
