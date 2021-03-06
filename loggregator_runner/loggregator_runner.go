package loggregator_runner

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/cloudfoundry/gunk/runner_support"
	. "github.com/onsi/gomega"
	"github.com/vito/cmdtest"
)

type LoggregatorRunner struct {
	loggregatorPath string
	configFile      *os.File

	loggregatorSession *cmdtest.Session

	Config
}

type Config struct {
	IncomingPort           int
	OutgoingPort           int
	MaxRetainedLogMessages int
	SharedSecret           string

	NatsHost string
	NatsPort int
}

func New(loggregatorPath string, config Config) *LoggregatorRunner {
	configFile, err := ioutil.TempFile(os.TempDir(), "loggregator-config")
	Ω(err).ShouldNot(HaveOccurred())

	defer configFile.Close()

	runner := &LoggregatorRunner{
		loggregatorPath: loggregatorPath,
		configFile:      configFile,

		Config: config,
	}

	err = json.NewEncoder(configFile).Encode(runner.Config)
	Ω(err).ShouldNot(HaveOccurred())

	return runner
}

func (runner *LoggregatorRunner) Start() {
	sess, err := cmdtest.StartWrapped(exec.Command(
		runner.loggregatorPath,
		"--config", runner.configFile.Name(),
		"--debug",
	), runner_support.TeeToGinkgoWriter, runner_support.TeeToGinkgoWriter)
	Ω(err).ShouldNot(HaveOccurred())

	runner.loggregatorSession = sess
}

func (runner *LoggregatorRunner) Stop() {
	err := runner.loggregatorSession.Cmd.Process.Signal(os.Interrupt)
	Ω(err).ShouldNot(HaveOccurred())
}
