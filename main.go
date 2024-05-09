package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "os"
    "sync"
    "time"

    "github.com/Azure/azure-sdk-for-go/sdk/cognitiveservices/speech/speechsdk"
    "github.com/Azure/go-autorest/autorest/azure/auth"
)

type Word struct {
    SpeakerId   string  `json:"SpeakerId,omitempty"`
    Word         string  `json:"Word,omitempty"`
    Offset       float64 `json:"Offset,omitempty"`
    Duration     float64 `json:"Duration,omitempty"`
}

func main() {
    subscriptionKey := os.Getenv("AZURE_SPEECH_KEY")
    if subscriptionKey == "" {
        fmt.Println("AZURE_SPEECH_KEY must be set")
        return
    }

    serviceRegion := os.Getenv("AZURE_SERVICE_REGION")
    if serviceRegion == "" {
        fmt.Println("AZURE_SERVICE_REGION must be set")
        return
    }

    audioFilename := os.Getenv("SOUND_FILE")
    if audioFilename == "" {
        fmt.Println("SOUND_FILE must be set")
        return
    }

    outputFilename := os.Getenv("OUTPUT_FILE")
    if outputFilename == "" {
        fmt.Println("OUTPUT_FILE must be set")
        return
    }

    config, err := speechsdk.NewSpeechConfig(subscriptionKey, serviceRegion)
    if err != nil {
        fmt.Printf("Error creating speech config: %v\n", err)
        return
    }

    config.SetSpeechRecognitionLanguage("en-US")
    config.SetOutputFormat(speechsdk.SpeechServiceResponse_Json)
    config.SetProperty("PunctuationMode", "DictatedAndAutomatic", "en-US")
    config.SetProperty("EnableWordLevelTimestamps", "true", "en-US")
    config.SetProperty("EnableDictation", "true", "en-US")
    config.SetProperty("EnableSpeakerRecognition", "true", "en-US")

    audioData, err := ioutil.ReadFile(audioFilename)
    if err != nil {
        fmt.Printf("Error reading audio file: %v\n", err)
        return
    }

    audioInput := speechsdk.NewAudioDataStream(audioData)
    recognizer := speechsdk.NewSpeechRecognizer(config, audioInput)

    transcriptData := &sync.Map{}
    speakers := &sync.Map{}

    recognizer.Recognized = func(s *speechsdk.SpeechRecognitionEventArgs) {
        if s.Result.Reason == speechsdk.ResultReason_RecognizedSpeech {
            var resultJson map[string]interface{}
            json.Unmarshal([]byte(s.Result.Text), &resultJson)

            nBest := resultJson["NBest"].([]interface{})
            for _, sentence := range nBest {
                words := sentence.(map[string]interface{})["Words"].([]interface{})
                for _, word := range words {
                    w := word.(map[string]interface{})
                    speakerId := w["SpeakerId"].(string)
                    if speakerId == "" {
                        speakerId = "Unknown"
                    }

                    speakerName := speakers.Load(speakerId)
                    if speakerName == nil {
                        speakers.Store(speakerId, fmt.Sprintf("Speaker %d", len(*speakers)+1))
                        speakerName = speakers.Load(speakerId)
                    }

                    startTime := time.Unix(0, int64(w["Offset"].(float64)*10000000)).(time.Time)
                    endTime := time.Unix(0, int64(w["Offset"].(float64)+w["Duration"].(float64))*10000000).(time.Time)

                    transcriptData.Store(fmt.Sprintf("%d-%d", startTime.UnixNano(), endTime.UnixNano()), &Word{
                        SpeakerId:   speakerId,
                        Word:         w["Word"].(string),
                        Offset:       w["Offset"].(float64),
                        Duration:     w["Duration"].(float64),
                    })

                    fmt.Printf("- **%s** (%s - %s): %s\n", speakerName, startTime.Format("04:05.000"), endTime.Format("04:05.000"), w["Word"].(string))
                }
            }
        }
    }

    fmt.Println("Starting transcription and diarization...")
    _, err = recognizer.RecognizeOnceAsync()
    if err != nil {
        fmt.Printf("Error recognizing speech: %v\n", err)
        return
    }

    fmt.Println("Transcription completed. Writing to output file...")
    outputFile, err := os.Create(outputFilename)
    if err != nil {
        fmt.Printf("Error creating output file: %v\n", err)
        return
    }
    defer outputFile.Close()

    for _, value := range transcriptData.Iter() {
        word := value.Value.(*Word)
        fmt.Fprintf(outputFile, "- **%s** (%s - %s): %s\n", speakers.Load(word.SpeakerId), time.Unix(0, int64(word.Offset*10000000)).(time.Time).Format("04:05.000"), time.Unix(0, int64(word.Offset*10000000+word.Duration*10000000)).(time.Time).Format("04:05.000"), word.Word)
    }

    fmt.Printf("Output written to %s\n", outputFilename)
}