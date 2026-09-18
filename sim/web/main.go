package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	uuid "github.com/google/uuid"
	"github.com/pkg/browser"
	dist "github.com/wowsims/wotlk/binary_dist"
	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	proto "github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/optimizer"

	googleProto "google.golang.org/protobuf/proto"
)

func init() {
	sim.RegisterAll()
}

var (
	Version  string
	outdated int
)

func main() {
	if Version == "" {
		Version = "development"
	}
	var useFS = flag.Bool("usefs", false, "Use local file system for client files. Set to true during development.")
	var wasm = flag.Bool("wasm", false, "Use wasm for sim instead of web server apis. Can only be used with usefs=true")
	var simName = flag.String("sim", "", "Name of simulator to launch (ex: balance_druid, elemental_shaman, etc)")
	var host = flag.String("host", "localhost:3333", "URL to host the interface on.")
	var launch = flag.Bool("launch", true, "auto launch browser")
	var skipVersionCheck = flag.Bool("nvc", false, "set true to skip version check")

	flag.Parse()

	fmt.Printf("Version: %s\n", Version)
	if !*skipVersionCheck && Version != "development" {
		go func() {
			resp, err := http.Get("https://api.github.com/repos/wowsims/wotlk/releases/latest")
			if err != nil {
				return
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return
			}

			result := struct {
				Tag  string `json:"tag_name"`
				URL  string `json:"html_url"`
				Name string `json:"name"`
			}{}
			if err := json.Unmarshal(body, &result); err != nil {
				return
			}

			if result.Tag != Version {
				outdated = 2
				fmt.Printf("New version of simulator available: %s\n\tDownload at: %s\n", result.Name, result.URL)
			} else {
				outdated = 1
			}
		}()
	}

	s := &server{
		progMut:         sync.RWMutex{},
		asyncProgresses: map[string]*asyncProgress{},
	}
	s.runServer(*useFS, *host, *launch, *simName, *wasm, bufio.NewReader(os.Stdin))
}

// Handlers to decode and handle each proto function
var handlers = map[string]apiHandler{
	"/raidSim": {msg: func() googleProto.Message { return &proto.RaidSimRequest{} }, handle: func(msg googleProto.Message) googleProto.Message {
		return core.RunRaidSim(msg.(*proto.RaidSimRequest))
	}},
	"/statWeights": {msg: func() googleProto.Message { return &proto.StatWeightsRequest{} }, handle: func(msg googleProto.Message) googleProto.Message {
		return core.StatWeights(msg.(*proto.StatWeightsRequest))
	}},
	"/computeStats": {msg: func() googleProto.Message { return &proto.ComputeStatsRequest{} }, handle: func(msg googleProto.Message) googleProto.Message {
		return core.ComputeStats(msg.(*proto.ComputeStatsRequest))
	}},
}

var asyncAPIHandlers = map[string]asyncAPIHandler{
	"/raidSimAsync": {msg: func() googleProto.Message { return &proto.RaidSimRequest{} }, handle: func(msg googleProto.Message, reporter chan *proto.ProgressMetrics) {
		core.RunRaidSimAsync(msg.(*proto.RaidSimRequest), reporter)
	}},
	"/statWeightsAsync": {msg: func() googleProto.Message { return &proto.StatWeightsRequest{} }, handle: func(msg googleProto.Message, reporter chan *proto.ProgressMetrics) {
		core.StatWeightsAsync(msg.(*proto.StatWeightsRequest), reporter)
	}},
	"/bulkSimAsync": {msg: func() googleProto.Message { return &proto.BulkSimRequest{} }, handle: func(msg googleProto.Message, reporter chan *proto.ProgressMetrics) {
		// TODO: we can use context's to cancel stuff.
		// We should have all the async APIs take in context and let it be cancelled via its async ID.
		core.RunBulkSimAsync(context.Background(), msg.(*proto.BulkSimRequest), reporter)
	}},
}

type server struct {
	progMut         sync.RWMutex
	asyncProgresses map[string]*asyncProgress

	// Progress id of the running optimization, "" when there's none. Only one runs at a time, since
	// one already keeps all cores but one busy.
	optimizerMut   sync.Mutex
	optimizerRunID string

	// Starts an optimization and must close the channel after the final result. Nil means
	// optimizer.RunAsync; tests swap in a fake.
	runOptimizer func(context.Context, *proto.OptimizeGearRequest, chan *proto.ProgressMetrics)
	// Cancels an optimization nobody has polled for this long, so a closed or reloaded tab doesn't
	// hold the slot until the run ends. 0 means defaultOptimizerAbandonAfter.
	optimizerAbandonAfter time.Duration
}

// net_worker.js polls every 500 ms; the rest is slack for throttled background tabs.
const defaultOptimizerAbandonAfter = 2 * time.Minute

type apiHandler struct {
	msg    func() googleProto.Message
	handle func(googleProto.Message) googleProto.Message
}
type asyncAPIHandler struct {
	msg    func() googleProto.Message
	handle func(googleProto.Message, chan *proto.ProgressMetrics)
}

type asyncProgress struct {
	id             string
	latestProgress atomic.Value
	// Stops the run, for the ones /cancelAsync can stop. Nil otherwise.
	cancel context.CancelFunc
	// UnixNano of the last /asyncProgress poll, or of the start before the first one.
	lastPolled atomic.Int64
}

// addNewSim registers a run under a new progress id. first is what /asyncProgress returns until the
// run reports; nil means empty progress.
func (s *server) addNewSim(cancel context.CancelFunc, first *proto.ProgressMetrics) *asyncProgress {
	if first == nil {
		first = &proto.ProgressMetrics{}
	}
	newID := uuid.NewString()
	simProgress := &asyncProgress{
		id:     newID,
		cancel: cancel,
	}
	simProgress.latestProgress.Store(first)
	simProgress.lastPolled.Store(time.Now().UnixNano())

	s.progMut.Lock()
	s.asyncProgresses[newID] = simProgress
	s.progMut.Unlock()

	return simProgress
}

func isFinalProgress(p *proto.ProgressMetrics) bool {
	return p.FinalRaidResult != nil || p.FinalWeightResult != nil || p.FinalBulkResult != nil || p.FinalOptimizeResult != nil
}

func writeAsyncAPIResult(w http.ResponseWriter, progressID string, status int) {
	outbytes, err := googleProto.Marshal(&proto.AsyncAPIResult{
		ProgressId: progressID,
	})
	if err != nil {
		log.Printf("[ERROR] Failed to marshal result: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/x-protobuf")
	w.WriteHeader(status)
	w.Write(outbytes)
}

func (s *server) handleAsyncAPI(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	endpoint := r.URL.Path
	handler, ok := asyncAPIHandlers[endpoint]
	if !ok {
		log.Printf("Invalid Endpoint: %s", endpoint)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	msg := handler.msg()
	if err := googleProto.Unmarshal(body, msg); err != nil {
		log.Printf("Failed to parse request: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// reporter channel is handed into the core simulation.
	//  as the simulation advances it will push changes to the channel
	//  these changes will be consumed by the goroutine below so the asyncProgress endpoint can fetch the results.
	reporter := make(chan *proto.ProgressMetrics, 100)
	handler.handle(msg, reporter)

	// Generate a new async simulation
	simProgress := s.addNewSim(nil, nil)

	// Now launch a background process that pulls progress reports off the reporter channel
	// and pushes it into the async progress cache.
	go func() {
		for {
			select {
			case <-time.After(time.Minute * 10):
				// if we get no progress after 10 minutes, delete the pending sim and exit.
				s.progMut.Lock()
				delete(s.asyncProgresses, simProgress.id)
				s.progMut.Unlock()
				return
			case progMetric := <-reporter:
				if progMetric == nil {
					return
				}
				simProgress.latestProgress.Store(progMetric)
				if isFinalProgress(progMetric) {
					return
				}
			}
		}
	}()

	writeAsyncAPIResult(w, simProgress.id, http.StatusOK)
}

// handleOptimizeGearAsync starts an optimization. While another one runs it answers 409 instead,
// still with a progress id: that id's final result carries the error, so async clients show it like
// any other failure.
func (s *server) handleOptimizeGearAsync(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	req := &proto.OptimizeGearRequest{}
	if err := googleProto.Unmarshal(body, req); err != nil {
		log.Printf("Failed to parse request: %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.Settings == nil {
		req.Settings = &proto.OptimizerSettings{}
	}
	// leave a core for the server and the other async routes
	maxWorkers := int32(max(1, runtime.GOMAXPROCS(0)-1))
	if req.Settings.Workers <= 0 || req.Settings.Workers > maxWorkers {
		req.Settings.Workers = maxWorkers
	}

	var ctx context.Context
	var cancel context.CancelFunc
	var simProgress *asyncProgress
	s.optimizerMut.Lock()
	runningID := s.optimizerRunID
	if runningID == "" {
		ctx, cancel = context.WithCancel(context.Background())
		simProgress = s.addNewSim(cancel, nil)
		s.optimizerRunID = simProgress.id
	}
	s.optimizerMut.Unlock()

	if simProgress == nil {
		refused := s.addNewSim(nil, &proto.ProgressMetrics{FinalOptimizeResult: &proto.OptimizerResult{
			ErrorResult: fmt.Sprintf("Another optimization is already running (progress id %s). Wait for it to finish, or cancel it.", runningID),
		}})
		writeAsyncAPIResult(w, refused.id, http.StatusConflict)
		return
	}

	run := s.runOptimizer
	if run == nil {
		run = optimizer.RunAsync
	}
	reporter := make(chan *proto.ProgressMetrics, 100)
	run(ctx, req, reporter)

	abandonAfter := s.optimizerAbandonAfter
	if abandonAfter <= 0 {
		abandonAfter = defaultOptimizerAbandonAfter
	}
	// No 10-minute timeout like the other routes: the run always sends a final result and closes
	// the channel, and the slot has to stay taken until it does. An abandoned run gets cancelled
	// instead, and its cancelled result is kept for a client that comes back.
	go func() {
		defer cancel()
		defer s.finishOptimizer(simProgress.id)
		check := time.NewTicker(abandonAfter / 4)
		defer check.Stop()
		for {
			select {
			case progMetric, ok := <-reporter:
				if !ok {
					return
				}
				if isFinalProgress(progMetric) {
					// free the slot before the result is visible, so whoever sees it can start the next run
					s.finishOptimizer(simProgress.id)
				}
				simProgress.latestProgress.Store(progMetric)
			case <-check.C:
				if ctx.Err() == nil && time.Since(time.Unix(0, simProgress.lastPolled.Load())) > abandonAfter {
					log.Printf("Cancelling optimization %s: nobody polled it for %s", simProgress.id, abandonAfter)
					cancel()
				}
			}
		}
	}()

	writeAsyncAPIResult(w, simProgress.id, http.StatusOK)
}

func (s *server) finishOptimizer(progressID string) {
	s.optimizerMut.Lock()
	defer s.optimizerMut.Unlock()
	if s.optimizerRunID == progressID {
		s.optimizerRunID = ""
	}
}

// handleCancelAsync stops the run with the AsyncAPIResult's progress id. The run still finishes
// through /asyncProgress, with its final result marked cancelled.
func (s *server) handleCancelAsync(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	msg := &proto.AsyncAPIResult{}
	if err := googleProto.Unmarshal(body, msg); err != nil {
		log.Printf("Failed to parse request: %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	s.progMut.RLock()
	progress, ok := s.asyncProgresses[msg.ProgressId]
	s.progMut.RUnlock()
	if !ok || progress.cancel == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	progress.cancel()
	w.WriteHeader(http.StatusOK)
}

func (s *server) setupAsyncServer(mux *http.ServeMux) {
	// All async handlers here will call the addNewSim, generating a new UUID and cached progress state.
	for route := range asyncAPIHandlers {
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			s.handleAsyncAPI(w, r)
		})
	}
	mux.HandleFunc("/optimizeGearAsync", s.handleOptimizeGearAsync)
	mux.HandleFunc("/cancelAsync", s.handleCancelAsync)

	// asyncProgress will fetch the current progress of a simulation by its UUID.
	mux.HandleFunc("/asyncProgress", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return
		}
		msg := &proto.AsyncAPIResult{}
		if err := googleProto.Unmarshal(body, msg); err != nil {
			log.Printf("Failed to parse request: %s", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Read lock the map of all progress statuses, fetching current one.
		s.progMut.RLock()
		progress, ok := s.asyncProgresses[msg.ProgressId]
		s.progMut.RUnlock()
		if !ok {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		progress.lastPolled.Store(time.Now().UnixNano())
		latest := progress.latestProgress.Load().(*proto.ProgressMetrics)
		outbytes, err := googleProto.Marshal(latest)
		if err != nil {
			log.Printf("[ERROR] Failed to marshal result: %s", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// If this was the last result, delete the cache for this simulation.
		if isFinalProgress(latest) {
			s.progMut.Lock()
			delete(s.asyncProgresses, msg.ProgressId)
			s.progMut.Unlock()
		}
		w.Header().Add("Content-Type", "application/x-protobuf")
		w.Write(outbytes)
	})
}

func (s *server) runServer(useFS bool, host string, launchBrowser bool, simName string, wasm bool, inputReader *bufio.Reader) {
	s.setupAsyncServer(http.DefaultServeMux)

	var fs http.Handler
	if useFS {
		log.Printf("Using local file system for development.")
		fs = http.FileServer(http.Dir("./dist"))
	} else {
		log.Printf("Embedded file server running.")
		fs = http.FileServer(http.FS(dist.FS))
	}

	for route := range handlers {
		http.HandleFunc(route, handleAPI)
	}

	http.HandleFunc("/version", func(resp http.ResponseWriter, req *http.Request) {
		msg := fmt.Sprintf(`{"version": "%s", "outdated": %d}`, Version, outdated)
		resp.Write([]byte(msg))
	})
	http.HandleFunc("/", func(resp http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/" {
			http.Redirect(resp, req, "/wotlk/", http.StatusPermanentRedirect)
			return
		}
		resp.Header().Add("Cache-Control", "no-cache")
		if strings.HasSuffix(req.URL.Path, ".wasm") {
			resp.Header().Set("Content-Type", "application/wasm")
		}
		if strings.HasSuffix(req.URL.Path, ".js") {
			resp.Header().Set("Content-Type", "application/javascript")
		}
		if !useFS || (useFS && !wasm) {
			if strings.HasSuffix(req.URL.Path, "sim_worker.js") {
				req.URL.Path = strings.Replace(req.URL.Path, "sim_worker.js", "net_worker.js", 1)
			}
		}
		fs.ServeHTTP(resp, req)
	})

	if launchBrowser {
		if strings.HasPrefix(host, ":") {
			host = "localhost" + host
		}
		url := fmt.Sprintf("http://%s/wotlk/%s", host, simName)
		log.Printf("Launching interface on %s", url)
		go func() {
			err := browser.OpenURL(url)
			if err != nil {
				fmt.Printf("Error launching browser: %#v\n", err.Error())
				fmt.Printf("You will need to manually open your web browser to %s\n", url)
			}
		}()
	}

	go func() {
		// Launch server!
		if err := http.ListenAndServe(host, nil); err != nil {
			log.Printf("Failed to shutdown server: %s", err)
			os.Exit(1)
		}
		log.Printf("Server shutdown successfully.")
		os.Exit(0)
	}()

	// used to read a CTRL+C
	c := make(chan os.Signal, 10)
	signal.Notify(c, syscall.SIGINT)

	go func() {
		<-c
		log.Printf("Shutting down")
		os.Exit(0)
	}()
	fmt.Printf("Enter Command... '?' for list\n")
	for {
		fmt.Printf("> ")
		text, err := inputReader.ReadString('\n')
		if err != nil {
			// block forever
			<-c
			os.Exit(-1)
		}
		if len(text) == 0 {
			continue
		}
		command := strings.TrimSpace(text)
		switch command {
		case "profile":
			filename := fmt.Sprintf("profile_%d.cpu", time.Now().Unix())
			fmt.Printf("Running profiling for 15 seconds, output to %s\n", filename)
			f, err := os.Create(filename)
			if err != nil {
				log.Fatal("could not create CPU profile: ", err)
			}
			if err := pprof.StartCPUProfile(f); err != nil {
				log.Fatal("could not start CPU profile: ", err)
			}
			go func() {
				time.Sleep(time.Second * 15)
				pprof.StopCPUProfile()
				f.Close()
				fmt.Printf("Profiling complete.\n> ")
			}()
		case "sims":
			s.progMut.RLock()
			fmt.Printf("Total Sims Running: %d\n", len(s.asyncProgresses))
			for _, v := range s.asyncProgresses {
				latest := (v.latestProgress.Load()).(*proto.ProgressMetrics)
				fmt.Printf("Process: %s (%d sims)\n\t  Progress: %d/%d\n", v.id, latest.TotalSims, latest.CompletedIterations, latest.TotalIterations)
			}
			s.progMut.RUnlock()
		case "quit":
			os.Exit(1)
		case "?":
			fmt.Printf("Commands:\n\tsims - Lists all active async sims running currently.\n\tprofile - start a CPU profile for debugging performance\n\tquit - exits\n\n")
		case "":
			// nothing.
		default:
			fmt.Printf("Unknown command: '%s'", command)
		}
	}
}

// handleAPI is generic handler for any api function using protos.
func handleAPI(w http.ResponseWriter, r *http.Request) {
	endpoint := r.URL.Path

	body, err := io.ReadAll(r.Body)
	if err != nil {

		return
	}
	handler, ok := handlers[endpoint]
	if !ok {
		log.Printf("Invalid Endpoint: %s", endpoint)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	msg := handler.msg()
	if err := googleProto.Unmarshal(body, msg); err != nil {
		log.Printf("Failed to parse request: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	result := handler.handle(msg)

	outbytes, err := googleProto.Marshal(result)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal result: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/x-protobuf")
	w.Write(outbytes)
}
