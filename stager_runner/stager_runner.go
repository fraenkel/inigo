package stager_runner

import (
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/cloudfoundry/gunk/runner_support"
	. "github.com/onsi/gomega"
	"github.com/vito/cmdtest"
	. "github.com/vito/cmdtest/matchers"
)

type StagerRunner struct {
	stagerBin     string
	etcdMachines  []string
	natsAddresses []string

	stagerSession *cmdtest.Session
	CompilerUrl   string
}

func New(stagerBin string, etcdMachines []string, natsAddresses []string) *StagerRunner {
	return &StagerRunner{
		stagerBin:     stagerBin,
		etcdMachines:  etcdMachines,
		natsAddresses: natsAddresses,
	}
}

func (r *StagerRunner) Start(args ...string) {
	stagerSession, err := cmdtest.StartWrapped(
		exec.Command(
			r.stagerBin,
			append([]string{
				"-etcdMachines", strings.Join(r.etcdMachines, ","),
				"-natsAddresses", strings.Join(r.natsAddresses, ","),
			}, args...)...,
		),
		runner_support.TeeToGinkgoWriter,
		runner_support.TeeToGinkgoWriter,
	)

	Ω(err).ShouldNot(HaveOccurred())
	Ω(stagerSession).Should(SayWithTimeout(
		"Listening for staging requests!",
		1*time.Second,
	))

	r.stagerSession = stagerSession
}

func (r *StagerRunner) Stop() {
	if r.stagerSession != nil {
		r.stagerSession.Cmd.Process.Signal(syscall.SIGTERM)
	}
}
