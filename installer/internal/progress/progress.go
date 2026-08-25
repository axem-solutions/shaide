package progress

import (
	"context"
	"io"
	"time"
)

const (
	DefaultTick        = 5 * time.Second
	DefaultReportEvery = 250 * time.Millisecond
)

type Phase string

const (
	PhaseExtracting  Phase = "Extracting"
	PhaseDownloading Phase = "Downloading"
	PhaseBuilding    Phase = "Building artifact"
	PhaseUploading   Phase = "Uploading artifact"
	PhaseImporting   Phase = "Importing artifact"
)

type Event struct {
	Phase   Phase
	Current string

	Bytes      int64
	TotalBytes int64

	Files      int
	TotalFiles int

	Percent int
	Done    bool
}

type Snapshot struct {
	Bytes int64
	Files int
}

type Reporter func(Event)

type Tracker struct {
	event      Event
	reporter   Reporter
	every      time.Duration
	lastReport time.Time
	stopped    bool
}

func NewTracker(phase Phase, current string, totalBytes int64, totalFiles int, reporter Reporter) *Tracker {
	return &Tracker{
		event: Event{
			Phase:      phase,
			Current:    current,
			TotalBytes: totalBytes,
			TotalFiles: totalFiles,
		},
		reporter: reporter,
	}
}

func (t *Tracker) Throttle(every time.Duration) *Tracker {
	t.every = every
	return t
}

func (t *Tracker) Start() {
	t.emit(false, true, false)
}

func (t *Tracker) MoveToPhase(phase Phase, current string, totalBytes int64, totalFiles int) {
	t.event.Phase = phase
	t.event.Current = current
	t.event.Bytes = 0
	t.event.TotalBytes = totalBytes
	t.event.Files = 0
	t.event.TotalFiles = totalFiles
	t.stopped = false

	t.emit(false, true, false)
}

func (t *Tracker) SetCurrent(current string) {
	if t.stopped {
		return
	}

	t.event.Current = current
	t.emit(false, true, false)
}

func (t *Tracker) AddBytes(bytes int64) {
	if bytes == 0 || t.stopped {
		return
	}

	t.event.Bytes += bytes
	t.clamp()

	t.emit(false, false, false)
}

func (t *Tracker) AddFiles(files int) {
	if files <= 0 || t.stopped {
		return
	}

	t.event.Files += files
	t.clamp()

	t.emit(false, false, false)
}

func (t *Tracker) SetSnapshot(snapshot Snapshot) {
	if t.stopped {
		return
	}

	if snapshot.Bytes > t.event.Bytes {
		t.event.Bytes = snapshot.Bytes
	}
	if snapshot.Files > t.event.Files {
		t.event.Files = snapshot.Files
	}

	t.clamp()
	t.emit(false, false, false)
}

func (t *Tracker) Finish() {
	if t.event.TotalBytes > 0 {
		t.event.Bytes = t.event.TotalBytes
	}
	if t.event.TotalFiles > 0 {
		t.event.Files = t.event.TotalFiles
	}

	t.stopped = true
	t.emit(true, true, true)
}

func (t *Tracker) Stop() {
	t.stopped = true
	t.emit(true, true, false)
}

func (t *Tracker) Poll(ctx context.Context, every time.Duration, sample func() Snapshot) {
	if sample == nil {
		return
	}

	if every <= 0 {
		every = DefaultTick
	}

	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			snapshot := sample()
			if snapshot.Bytes == 0 && snapshot.Files == 0 {
				continue
			}
			t.SetSnapshot(snapshot)

		case <-ctx.Done():
			return
		}
	}
}

func (t *Tracker) Reader(reader io.Reader) io.Reader {
	return &countingReader{
		reader:  reader,
		tracker: t,
	}
}

func (t *Tracker) clamp() {
	if t.event.Bytes < 0 {
		t.event.Bytes = 0
	}

	if t.event.TotalBytes > 0 && t.event.Bytes > t.event.TotalBytes {
		t.event.Bytes = t.event.TotalBytes
	}

	if t.event.Files < 0 {
		t.event.Files = 0
	}

	if t.event.TotalFiles > 0 && t.event.Files > t.event.TotalFiles {
		t.event.Files = t.event.TotalFiles
	}
}

func (t *Tracker) emit(done bool, force bool, complete bool) {
	if t.reporter == nil {
		return
	}

	now := time.Now()
	if !force && t.every > 0 && now.Sub(t.lastReport) < t.every {
		return
	}

	event := t.event
	event.Done = done
	event.Percent = eventPercent(event)

	if complete {
		event.Percent = 100
	}

	t.lastReport = now
	t.reporter(event)
}

type countingReader struct {
	reader  io.Reader
	tracker *Tracker
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.tracker.AddBytes(int64(n))
	}

	return n, err
}

func eventPercent(event Event) int {
	if event.TotalBytes > 0 {
		return Percent(event.Bytes, event.TotalBytes)
	}

	if event.TotalFiles > 0 {
		return Percent(int64(event.Files), int64(event.TotalFiles))
	}

	return 0
}

func Percent(done, total int64) int {
	if total <= 0 || done <= 0 {
		return 0
	}

	percent := done * 100 / total
	if percent > 100 {
		return 100
	}

	return int(percent)
}
