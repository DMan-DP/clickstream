package publisher

import "testing"

func TestKafkaTimeouts(t *testing.T) {
	attempt := dialTimeout + writeTimeout + readTimeout
	worstCase := (maxRetry+1)*attempt + maxRetry*(retryBackoff+metadataTimeout)

	t.Run("worst case", func(t *testing.T) {
		if worstCase > sendBudget {
			t.Errorf("worst case %v exceeds sendBudget %v; lower the sarama timeouts or retries", worstCase, sendBudget)
		}
	})

	t.Run("broker timeout", func(t *testing.T) {
		if producerTimeout >= readTimeout {
			t.Errorf("producerTimeout %v must be less than readTimeout %v, otherwise the client gives up before the broker answers", producerTimeout, readTimeout)
		}
	})

	t.Run("metadata timeout", func(t *testing.T) {
		if metadataTimeout < attempt {
			t.Errorf("metadataTimeout %v is shorter than one attempt %v", metadataTimeout, attempt)
		}
	})
}
