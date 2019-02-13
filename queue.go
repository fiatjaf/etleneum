package main

import (
	"encoding/json"
	"net/url"
	"time"

	"github.com/adjust/redismq"
	"github.com/tidwall/gjson"
)

func openQueue(queueName string) *redismq.Queue {
	rurl, _ := url.Parse(s.RedisURL)
	pw, _ := rurl.User.Password()
	return redismq.CreateQueue(rurl.Hostname(), rurl.Port(), pw, 0, queueName)
}

func startQueue() {
	queue := openQueue("tasks")

	consumer, err := queue.AddConsumer("soloconsumer")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create consumer")
	}

	go func() {
		for {
			var pack *redismq.Package

			if p, err := consumer.Get(); err != nil {
				log.Error().Err(err).Msg("failed to get package from queue")
			} else {
				pack = p
			}

			packj := gjson.Parse(pack.Payload)
			switch packj.Get("kind").String() {
			case "payment-from-call":
				bolt11 := packj.Get("bolt11").String()
				callid := packj.Get("callid").String()
				ctid := packj.Get("ctid").String()

				var res gjson.Result
				res, err = ln.CallWithCustomTimeout(time.Second*10, "pay", bolt11)
				log.Debug().Err(err).Str("res", res.String()).
					Str("bolt11", bolt11).
					Str("callid", callid).
					Str("ctid", ctid).
					Msg("payment from contract call")
			}

			if err != nil {
				if err := pack.Fail(); err != nil {
					log.Error().Err(err).Str("payload", pack.Payload).
						Msg("failed to fail package")
				}
			} else {
				if err := pack.Ack(); err != nil {
					log.Error().Err(err).Str("payload", pack.Payload).
						Msg("failed to ack package")
				}
			}
		}
	}()
}

func queueTask(kind string, payload map[string]interface{}) error {
	queue := openQueue("tasks")

	payload["kind"] = kind
	taskpayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return queue.Put(string(taskpayload))
}
