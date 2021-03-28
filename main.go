package main

import (
	"context"
	"fmt"
	"github.com/kubemq-io/kubemq-go"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"log"
	"sync"
	"time"
)

type Config struct {
	Address string
	Queue   string
	Send    int
	Threads int
	Rounds  int
}

var (
	_ = pflag.String("address", "localhost", "kubemq-address")
	_ = pflag.String("queue", "q1", "queue destination")
	_ = pflag.Int("send", 1000, "total send messages")
	_ = pflag.Int("threads", 1, "total threads")
	_ = pflag.Int("rounds", 0, "total rounds")
)

func LoadConfig() (*Config, error) {

	pflag.Parse()
	cfg := &Config{}

	_ = viper.BindEnv("Address", "ADDRESS")
	_ = viper.BindEnv("Send", "SEND")
	_ = viper.BindEnv("Threads", "THREADS")
	_ = viper.BindEnv("Queue", "QUEUE")
	_ = viper.BindEnv("Rounds", "ROUNDS")

	_ = viper.BindPFlag("Address", pflag.CommandLine.Lookup("address"))
	_ = viper.BindPFlag("Queue", pflag.CommandLine.Lookup("queue"))
	_ = viper.BindPFlag("Send", pflag.CommandLine.Lookup("send"))
	_ = viper.BindPFlag("Threads", pflag.CommandLine.Lookup("threads"))
	_ = viper.BindPFlag("Rounds", pflag.CommandLine.Lookup("rounds"))

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
func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sender, err := kubemq.NewClient(ctx,
		kubemq.WithAddress(cfg.Address, 50000),
		kubemq.WithClientId("stream-queue-sender"),
		kubemq.WithTransportType(kubemq.TransportTypeGRPC))
	if err != nil {
		log.Fatal(err)

	}
	defer sender.Close()
	var roundList []float64
	cnt := 0
	for {
		cnt++
		if cfg.Rounds > 0 && cnt > cfg.Rounds {
			break
		}
		for i := 0; i < cfg.Threads; i++ {
			channel := fmt.Sprintf("%s%d", cfg.Queue, i)
			batch := sender.NewQueueMessages()
			for i := 0; i < cfg.Send; i++ {
				batch.Add(kubemq.NewQueueMessage().SetChannel(channel).SetBody([]byte("some-stream-simple-queue-message")))
			}
			_, err = batch.Send(ctx)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf(fmt.Sprintf("%d messages sent to queue %s", cfg.Send, channel))
		}

		startAll := time.Now().UnixNano()
		wg := sync.WaitGroup{}
		wg.Add(cfg.Threads)
		for i := 0; i < cfg.Threads; i++ {
			go func(index int) {
				defer wg.Done()
				receiver, err := kubemq.NewClient(ctx,
					kubemq.WithAddress(cfg.Address, 50000),
					kubemq.WithClientId(fmt.Sprintf("stream-queue-receiver-%d", index)),
					kubemq.WithTransportType(kubemq.TransportTypeGRPC))
				if err != nil {
					log.Fatal(err)

				}
				defer receiver.Close()
				channel := fmt.Sprintf("%s%d", cfg.Queue, index)

				for i := 0; i < cfg.Send; i++ {

					stream := receiver.NewStreamQueueMessage().SetChannel(channel)

					// get message from the queue
					msg, err := stream.Next(ctx, 15, 10)
					if err != nil {
						log.Fatal(err)
					}

					err = msg.Ack()
					if err != nil {
						log.Fatal(err)
					}
					stream.Close()
				}
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