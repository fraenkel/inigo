package inigo_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudfoundry-incubator/inigo/executor_runner"
	"github.com/cloudfoundry-incubator/inigo/inigolistener"
	"github.com/cloudfoundry-incubator/inigo/loggredile"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/cloudfoundry-incubator/runtime-schema/models/factories"
	"github.com/cloudfoundry/loggregatorlib/logmessage"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/vito/cmdtest/matchers"
)

var _ = Describe("Executor", func() {
	var bbs *Bbs.BBS

	BeforeEach(func() {
		bbs = Bbs.New(etcdRunner.Adapter())
	})

	Describe("starting without a snaphsot", func() {
		It("should come up, just fine", func() {
			executorRunner.Start()
			executorRunner.Stop()
		})
	})

	Describe("when starting with invalid memory/disk", func() {
		It("should exit with failure", func() {
			executorRunner.StartWithoutCheck(executor_runner.Config{MemoryMB: -1, DiskMB: -1, SnapshotFile: "/tmp/i_dont_exist"})
			Ω(executorRunner.Session).Should(SayWithTimeout("valid memory and disk capacity must be specified", time.Second))
			Ω(executorRunner.Session).Should(ExitWith(1))
		})
	})

	Describe("when restarted with running tasks", func() {
		var tmpdir string
		var registrySnapshotFile string
		var executorConfig executor_runner.Config

		BeforeEach(func() {
			tmpdir, err := ioutil.TempDir(os.TempDir(), "executor-registry")
			Ω(err).ShouldNot(HaveOccurred())

			registrySnapshotFile = filepath.Join(tmpdir, "snapshot.json")

			executorConfig = executor_runner.Config{
				MemoryMB:     1024,
				DiskMB:       1024,
				SnapshotFile: registrySnapshotFile,
			}

			executorRunner.Start(executorConfig)

			existingGuid := factories.GenerateGuid()

			existingRunOnce := factories.BuildRunOnceWithRunAction(
				1024,
				1024,
				inigolistener.CurlCommand(existingGuid)+"; sleep 60",
			)

			bbs.DesireRunOnce(existingRunOnce)

			Eventually(inigolistener.ReportingGuids, 5.0).Should(ContainElement(existingGuid))

			executorRunner.Stop()
		})

		AfterEach(func() {
			executorRunner.Stop()

			os.RemoveAll(tmpdir)
		})

		It("retains their resource usage", func() {
			executorRunner.Start(executorConfig)

			cantFitGuid := factories.GenerateGuid()

			cantFitRunOnce := factories.BuildRunOnceWithRunAction(
				1024,
				1024,
				inigolistener.CurlCommand(cantFitGuid)+"; sleep 60",
			)

			bbs.DesireRunOnce(cantFitRunOnce)

			Consistently(inigolistener.ReportingGuids, 5.0).ShouldNot(ContainElement(cantFitGuid))
		})

		Context("and we were previously running more than we can now handle", func() {
			Context("of memory", func() {
				It("fails to start with a helpful message", func() {
					executorConfig.MemoryMB = 512

					executorRunner.StartWithoutCheck(executorConfig)

					Ω(executorRunner.Session).Should(SayWithTimeout(
						"memory requirements in snapshot exceed",
						time.Second,
					))

					Ω(executorRunner.Session).Should(ExitWith(1))
				})
			})

			Context("of disk", func() {
				It("fails to start with a helpful message", func() {
					executorConfig.DiskMB = 512

					executorRunner.StartWithoutCheck(executorConfig)

					Ω(executorRunner.Session).Should(SayWithTimeout(
						"disk requirements in snapshot exceed",
						time.Second,
					))

					Ω(executorRunner.Session).Should(ExitWith(1))
				})
			})
		})

		Context("when the snapshot is corrupted", func() {
			It("should exit with failure", func() {
				file, err := ioutil.TempFile(os.TempDir(), "executor-invalid-snapshot")
				Ω(err).ShouldNot(HaveOccurred())

				_, err = file.Write([]byte("ß"))
				Ω(err).ShouldNot(HaveOccurred())

				executorConfig.SnapshotFile = file.Name()

				executorRunner.StartWithoutCheck(executorConfig)

				Ω(executorRunner.Session).Should(SayWithTimeout("corrupt registry", time.Second))
				Ω(executorRunner.Session).Should(ExitWith(1))
			})
		})
	})

	Describe("Heartbeating", func() {
		It("should heartbeat its presence", func() {
			executorRunner.Start()

			Eventually(func() interface{} {
				executors, _ := bbs.GetAllExecutors()
				return executors
			}).Should(HaveLen(1))
		})
	})

	Describe("Resource limits", func() {
		BeforeEach(func() {
			executorRunner.Start()
		})

		It("should only pick up tasks if it has capacity", func() {
			firstGuyGuid := factories.GenerateGuid()
			secondGuyGuid := factories.GenerateGuid()
			firstGuyRunOnce := factories.BuildRunOnceWithRunAction(1024, 1024, inigolistener.CurlCommand(firstGuyGuid)+"; sleep 5")
			bbs.DesireRunOnce(firstGuyRunOnce)

			Eventually(inigolistener.ReportingGuids, 5.0).Should(ContainElement(firstGuyGuid))

			secondGuyRunOnce := factories.BuildRunOnceWithRunAction(1024, 1024, inigolistener.CurlCommand(secondGuyGuid))
			bbs.DesireRunOnce(secondGuyRunOnce)

			Consistently(inigolistener.ReportingGuids, 2.0).ShouldNot(ContainElement(secondGuyGuid))
		})
	})

	Describe("Stack", func() {
		BeforeEach(func() {
			executorRunner.Start(executor_runner.Config{Stack: "penguin"})
		})

		It("should only pick up tasks if the stacks match", func() {
			matchingGuid := factories.GenerateGuid()
			matchingRunOnce := factories.BuildRunOnceWithRunAction(1, 1, inigolistener.CurlCommand(matchingGuid)+"; sleep 10")
			matchingRunOnce.Stack = "penguin"

			nonMatchingGuid := factories.GenerateGuid()
			nonMatchingRunOnce := factories.BuildRunOnceWithRunAction(1, 1, inigolistener.CurlCommand(nonMatchingGuid)+"; sleep 10")
			nonMatchingRunOnce.Stack = "lion"

			bbs.DesireRunOnce(matchingRunOnce)
			bbs.DesireRunOnce(nonMatchingRunOnce)

			Consistently(inigolistener.ReportingGuids, 2.0).ShouldNot(ContainElement(nonMatchingGuid), "Did not expect to see this app running, as it has the wrong stack.")
			Eventually(inigolistener.ReportingGuids, 5.0).Should(ContainElement(matchingGuid))
		})
	})

	Describe("Running a command", func() {
		var guid string
		BeforeEach(func() {
			executorRunner.Start()
			guid = factories.GenerateGuid()
		})

		It("should run the command with the provided environment", func() {
			env := [][]string{
				{"FOO", "BAR"},
				{"BAZ", "WIBBLE"},
				{"FOO", "$FOO-$BAZ"},
			}
			runOnce := models.RunOnce{
				Guid:     factories.GenerateGuid(),
				MemoryMB: 1024,
				DiskMB:   1024,
				Actions: []models.ExecutorAction{
					{Action: models.RunAction{Script: `echo $FOO > out`, Env: env}},
					{Action: models.UploadAction{From: "out", To: inigolistener.UploadUrl("out")}},
					{Action: models.RunAction{Script: inigolistener.CurlCommand(guid)}},
				},
			}

			bbs.DesireRunOnce(runOnce)

			Eventually(inigolistener.ReportingGuids, 5.0).Should(ContainElement(guid))
			Ω(inigolistener.DownloadFileString("out")).Should(Equal("BAR-WIBBLE\n"))

			Eventually(bbs.GetAllCompletedRunOnces, 5.0).Should(HaveLen(1))
			runOnces, _ := bbs.GetAllCompletedRunOnces()
			Ω(runOnces[0].Failed).Should(BeFalse())
		})

		Context("when the command times out", func() {
			It("should fail the RunOnce", func() {
				runOnce := models.RunOnce{
					Guid:     factories.GenerateGuid(),
					MemoryMB: 1024,
					DiskMB:   1024,
					Actions: []models.ExecutorAction{
						{Action: models.RunAction{Script: inigolistener.CurlCommand(guid)}},
						{Action: models.RunAction{Script: `sleep 0.8`, Timeout: 500 * time.Millisecond}},
					},
				}

				bbs.DesireRunOnce(runOnce)

				Eventually(inigolistener.ReportingGuids, 5.0).Should(ContainElement(guid))
				Eventually(bbs.GetAllCompletedRunOnces, 5.0).Should(HaveLen(1))
				runOnces, _ := bbs.GetAllCompletedRunOnces()
				Ω(runOnces[0].Failed).Should(BeTrue())
				Ω(runOnces[0].FailureReason).Should(ContainSubstring("timed out"))
			})
		})
	})

	Describe("Running a downloaded file", func() {
		var guid string
		BeforeEach(func() {
			executorRunner.Start()

			guid = factories.GenerateGuid()
			inigolistener.UploadFileString("curling.sh", inigolistener.CurlCommand(guid))
		})

		It("downloads the file", func() {
			runOnce := models.RunOnce{
				Guid:     factories.GenerateGuid(),
				MemoryMB: 1024,
				DiskMB:   1024,
				Actions: []models.ExecutorAction{
					{Action: models.DownloadAction{From: inigolistener.DownloadUrl("curling.sh"), To: "curling.sh", Extract: false}},
					{Action: models.RunAction{Script: "bash curling.sh"}},
				},
			}

			bbs.DesireRunOnce(runOnce)

			Eventually(inigolistener.ReportingGuids, 5.0).Should(ContainElement(guid))
		})
	})

	Describe("Uploading a file", func() {
		var guid string
		BeforeEach(func() {
			executorRunner.Start()

			guid = factories.GenerateGuid()
		})

		It("uploads the file", func() {
			runOnce := models.RunOnce{
				Guid:     factories.GenerateGuid(),
				MemoryMB: 1024,
				DiskMB:   1024,
				Actions: []models.ExecutorAction{
					{Action: models.RunAction{Script: `echo "tasty thingy" > thingy`}},
					{Action: models.UploadAction{From: "thingy", To: inigolistener.UploadUrl("thingy")}},
					{Action: models.RunAction{Script: inigolistener.CurlCommand(guid)}},
				},
			}

			bbs.DesireRunOnce(runOnce)

			Eventually(inigolistener.ReportingGuids, 5.0).Should(ContainElement(guid))
			Ω(inigolistener.DownloadFileString("thingy")).Should(Equal("tasty thingy\n"))
		})
	})

	Describe("Fetching results", func() {
		BeforeEach(func() {
			executorRunner.Start()
		})

		It("should fetch the contents of the requested file and provide the content in the completed RunOnce", func() {
			runOnce := models.RunOnce{
				Guid:     factories.GenerateGuid(),
				MemoryMB: 1024,
				DiskMB:   1024,
				Actions: []models.ExecutorAction{
					{Action: models.RunAction{Script: `echo "tasty thingy" > thingy`}},
					{Action: models.FetchResultAction{File: "thingy"}},
				},
			}

			bbs.DesireRunOnce(runOnce)

			Eventually(bbs.GetAllCompletedRunOnces, 5.0).Should(HaveLen(1))

			runOnces, _ := bbs.GetAllCompletedRunOnces()
			Ω(runOnces[0].Result).Should(Equal("tasty thingy\n"))
		})
	})

	Describe("A RunOnce with logging configured", func() {
		BeforeEach(func() {
			executorRunner.Start()
		})

		It("has its stdout and stderr emitted to Loggregator", func(done Done) {
			logGuid := factories.GenerateGuid()

			messages, stop := loggredile.StreamMessages(
				loggregatorRunner.Config.OutgoingPort,
				"/tail/?app="+logGuid,
			)

			runOnce := factories.BuildRunOnceWithRunAction(
				1024,
				1024,
				"echo out A; echo out B; echo out C; echo err A 1>&2; echo err B 1>&2; echo err C 1>&2",
			)
			runOnce.Log.Guid = logGuid
			runOnce.Log.SourceName = "APP"

			bbs.DesireRunOnce(runOnce)

			outStream := []string{}
			errStream := []string{}

			for i := 0; i < 6; i++ {
				message := <-messages
				switch message.GetMessageType() {
				case logmessage.LogMessage_OUT:
					outStream = append(outStream, string(message.GetMessage()))
				case logmessage.LogMessage_ERR:
					errStream = append(errStream, string(message.GetMessage()))
				}
			}

			Ω(outStream).Should(Equal([]string{"out A", "out B", "out C"}))
			Ω(errStream).Should(Equal([]string{"err A", "err B", "err C"}))

			close(stop)
			close(done)
		}, 10.0)
	})
})
