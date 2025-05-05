package main

import (
	"container/heap"
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

type Task interface {
	ID() string
	Priority() int
	Run(ctx context.Context) error
}

type BaseTask struct {
	id       string
	priority int
	action   func(context.Context) error
}

func (t *BaseTask) ID() string    { return t.id }
func (t *BaseTask) Priority() int { return t.priority }
func (t *BaseTask) Run(ctx context.Context) error {
	return t.action(ctx)
}

type taskItem struct {
	task     Task
	index    int
	priority int
}

type PriorityQueue []*taskItem

func (pq PriorityQueue) Len() int { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].priority > pq[j].priority
}
func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}
func (pq *PriorityQueue) Push(x any) {
	n := len(*pq)
	item := x.(*taskItem)
	item.index = n
	*pq = append(*pq, item)
}
func (pq *PriorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

type Scheduler struct {
	pq       PriorityQueue
	lock     sync.Mutex
	cond     *sync.Cond
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	running  bool
	workers  int
	statsCh  chan string
	statsMap map[string]int
}

func NewScheduler(workers int) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		pq:       make(PriorityQueue, 0),
		ctx:      ctx,
		cancel:   cancel,
		workers:  workers,
		statsCh:  make(chan string, 100),
		statsMap: make(map[string]int),
	}
	s.cond = sync.NewCond(&s.lock)
	return s
}

func (s *Scheduler) Submit(t Task) {
	s.lock.Lock()
	defer s.lock.Unlock()
	heap.Push(&s.pq, &taskItem{task: t, priority: t.Priority()})
	s.cond.Signal()
}

func (s *Scheduler) worker(id int) {
	defer s.wg.Done()
	for {
		s.lock.Lock()
		for len(s.pq) == 0 && s.ctx.Err() == nil {
			s.cond.Wait()
		}
		if s.ctx.Err() != nil {
			s.lock.Unlock()
			return
		}
		item := heap.Pop(&s.pq).(*taskItem)
		s.lock.Unlock()

		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
		err := item.task.Run(ctx)
		cancel()
		if err != nil {
			log.Printf("Worker %d: task %s failed: %v", id, item.task.ID(), err)
		} else {
			log.Printf("Worker %d: task %s completed", id, item.task.ID())
		}
		s.statsCh <- item.task.ID()
	}
}

func (s *Scheduler) Start() {
	if s.running {
		return
	}
	s.running = true
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.worker(i)
	}
	go s.collectStats()
}

func (s *Scheduler) Stop() {
	s.cancel()
	s.cond.Broadcast()
	s.wg.Wait()
	close(s.statsCh)
}

func (s *Scheduler) collectStats() {
	for id := range s.statsCh {
		s.lock.Lock()
		s.statsMap[id]++
		s.lock.Unlock()
	}
}

func (s *Scheduler) PrintStats() {
	s.lock.Lock()
	defer s.lock.Unlock()
	fmt.Println("\n--- Task Completion Stats ---")
	for id, count := range s.statsMap {
		fmt.Printf("Task %s completed %d times\n", id, count)
	}
	fmt.Println("------------------------------")
}

// demoTask returns a Task that waits a random amount of time
func demoTask(id string, priority int) Task {
	return &BaseTask{
		id:       id,
		priority: priority,
		action: func(ctx context.Context) error {
			d := time.Duration(rand.Intn(1000)+100) * time.Millisecond
			select {
			case <-time.After(d):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())

	s := NewScheduler(5)
	s.Start()

	for i := 0; i < 100; i++ {
		t := demoTask(fmt.Sprintf("task-%02d", i), rand.Intn(10))
		s.Submit(t)
	}

	time.Sleep(5 * time.Second)
	s.Stop()
	s.PrintStats()
}
