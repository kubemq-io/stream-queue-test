package main

import (
	"context"
	"fmt"
	"github.com/kubemq-io/kubemq-go/clients"

	"github.com/kubemq-io/kubemq-go/clients/queues"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/atomic"
	"log"
	"math/rand"
	"sync"
	"time"
)

type Config struct {
	Address  string
	Queue    string
	Send     int
	Threads  int
	Rounds   int
	AckDelay int
}

var (
	_ = pflag.String("address", "localhost", "kubemq-address")
	_ = pflag.String("queue", "d", "queue destination")
	_ = pflag.Int("send", 3000, "total send messages")
	_ = pflag.Int("threads", 150, "total threads")
	_ = pflag.Int("rounds", 0, "total rounds")
	_ = pflag.Int("ack_delay", 0, "ack delay in seconds")
)

func LoadConfig() (*Config, error) {

	pflag.Parse()
	cfg := &Config{}

	_ = viper.BindEnv("Address", "ADDRESS")
	_ = viper.BindEnv("Send", "SEND")
	_ = viper.BindEnv("Threads", "THREADS")
	_ = viper.BindEnv("Queue", "QUEUE")
	_ = viper.BindEnv("Rounds", "ROUNDS")
	_ = viper.BindEnv("AckDelay", "ACK_DELAY")

	_ = viper.BindPFlag("Address", pflag.CommandLine.Lookup("address"))
	_ = viper.BindPFlag("Queue", pflag.CommandLine.Lookup("queue"))
	_ = viper.BindPFlag("Send", pflag.CommandLine.Lookup("send"))
	_ = viper.BindPFlag("Threads", pflag.CommandLine.Lookup("threads"))
	_ = viper.BindPFlag("Rounds", pflag.CommandLine.Lookup("rounds"))
	_ = viper.BindPFlag("AckDelay", pflag.CommandLine.Lookup("ack_delay"))

	err := viper.Unmarshal(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
func getFloatAvg(list []float64) float64 {
	cnt := 0.0
	for _, val := range list {
		cnt += val
	}
	return cnt / float64(len(list))
}
func getClient(ctx context.Context, host string, port int) *queues.QueuesClient {
	c, _ := queues.NewQueuesClient(ctx,
		clients.WithAddress(host, port),
		clients.WithClientId("stream-queue-sender"))

	return c
}
func main() {
	cfg, err := LoadConfig()
	rand.Seed(time.Now().UnixNano())
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sender := getClient(ctx, cfg.Address, 50000)
	defer func() {
		_ = sender.Close()
	}()
	var roundList []float64
	cnt := 0
	for {
		cnt++
		if cfg.Rounds > 0 && cnt > cfg.Rounds {
			break
		}
		for i := 0; i < cfg.Threads; i++ {
			channel := fmt.Sprintf("%s%d", cfg.Queue, i)
			var batch []*queues.QueueMessage
			for i := 0; i < cfg.Send; i++ {
				batch = append(batch, queues.NewQueueMessage().
					SetChannel(channel).SetBody([]byte("some-stream-simple-queue-message")))
			}
			_, err = sender.Send(ctx, batch...)
			if err != nil {
				log.Println(err)
			}

		}
		log.Printf(fmt.Sprintf("%d messages sent to queues", cfg.Threads*cfg.Send))
		startAll := time.Now().UnixNano()
		wg := sync.WaitGroup{}
		wg.Add(cfg.Threads)
		for i := 0; i < cfg.Threads; i++ {
			go func(index int) {
				defer wg.Done()
				cnt := atomic.NewInt32(0)
				channel := fmt.Sprintf("%s%d", cfg.Queue, index)
				response, err := sender.Poll(ctx, queues.NewPollRequest().
					SetChannel(channel).
					SetMaxItems(cfg.Send).
					SetAutoAck(false).
					SetWaitTimeout(10000).SetOnErrorFunc(func(err error) {
					log.Println(err)
				}))
				if err != nil {
					log.Println(err)
					return
				}
				if err := response.AckAll(); err != nil {
					log.Println(err)
					return
				}
				cnt.Add(int32(len(response.Messages)))
			}(i)
		}
		wg.Wait()
		avgTime := float64(time.Now().UnixNano()-startAll) / 1e6
		roundList = append(roundList, avgTime)
		fmt.Println(fmt.Sprintf("Round %d, %d Messages received, overall avg time: %f ms", cnt, cfg.Send*cfg.Threads, getFloatAvg(roundList)))
		time.Sleep(time.Second)
	}
	<-ctx.Done()
}
