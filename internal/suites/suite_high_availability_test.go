package suites

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type HighAvailabilityWebDriverSuite struct {
	*RodSuite
}

func NewHighAvailabilityWebDriverSuite() *HighAvailabilityWebDriverSuite {
	return &HighAvailabilityWebDriverSuite{RodSuite: new(RodSuite)}
}

func (s *HighAvailabilityWebDriverSuite) SetupSuite() {
	browser, err := StartRod()

	if err != nil {
		log.Fatal(err)
	}

	s.RodSession = browser
}

func (s *HighAvailabilityWebDriverSuite) TearDownSuite() {
	err := s.RodSession.Stop()

	if err != nil {
		log.Fatal(err)
	}
}

func (s *HighAvailabilityWebDriverSuite) SetupTest() {
	s.Page = s.doCreateTab(s.T(), HomeBaseURL)
	s.verifyIsHome(s.T(), s.Page)
}

func (s *HighAvailabilityWebDriverSuite) TearDownTest() {
	s.collectCoverage(s.Page)
	s.MustClose()
}

func (s *HighAvailabilityWebDriverSuite) TestShouldKeepUserSessionActive() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer func() {
		cancel()
		s.collectScreenshot(ctx.Err(), s.Page)
	}()

	secret := s.doRegisterThenLogout(s.T(), s.Context(ctx), "john", "password")

	err := haDockerEnvironment.Restart("redis-node-0")
	s.Require().NoError(err)

	s.doLoginTwoFactor(s.T(), s.Context(ctx), "john", "password", false, secret, "")
	s.verifyIsSecondFactorPage(s.T(), s.Context(ctx))
}

func (s *HighAvailabilityWebDriverSuite) TestShouldKeepUserSessionActiveWithPrimaryRedisNodeFailure() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer func() {
		cancel()
		s.collectScreenshot(ctx.Err(), s.Page)
	}()

	secret := s.doRegisterThenLogout(s.T(), s.Context(ctx), "john", "password")

	s.doLoginTwoFactor(s.T(), s.Context(ctx), "john", "password", false, secret, "")
	s.verifyIsSecondFactorPage(s.T(), s.Context(ctx))

	err := haDockerEnvironment.Stop("redis-node-0")
	s.Require().NoError(err)

	defer func() {
		err = haDockerEnvironment.Start("redis-node-0")
		s.Require().NoError(err)
	}()

	s.doVisit(s.T(), s.Context(ctx), HomeBaseURL)
	s.verifyIsHome(s.T(), s.Context(ctx))

	// Verify the user is still authenticated.
	s.doVisit(s.T(), s.Context(ctx), GetLoginBaseURL())
	s.verifyIsSecondFactorPage(s.T(), s.Context(ctx))

	// Then logout and login again to check we can see the secret.
	s.doLogout(s.T(), s.Context(ctx))
	s.verifyIsFirstFactorPage(s.T(), s.Context(ctx))

	s.doLoginTwoFactor(s.T(), s.Context(ctx), "john", "password", false, secret, fmt.Sprintf("%s/secret.html", SecureBaseURL))
	s.verifySecretAuthorized(s.T(), s.Context(ctx))
}

func (s *HighAvailabilityWebDriverSuite) TestShouldKeepUserSessionActiveWithPrimaryRedisSentinelFailureAndSecondaryRedisNodeFailure() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer func() {
		cancel()
		s.collectScreenshot(ctx.Err(), s.Page)
	}()

	secret := s.doRegisterThenLogout(s.T(), s.Context(ctx), "john", "password")

	s.doLoginTwoFactor(s.T(), s.Context(ctx), "john", "password", false, secret, "")
	s.verifyIsSecondFactorPage(s.T(), s.Context(ctx))

	err := haDockerEnvironment.Stop("redis-sentinel-0")
	s.Require().NoError(err)

	defer func() {
		err = haDockerEnvironment.Start("redis-sentinel-0")
		s.Require().NoError(err)
	}()

	err = haDockerEnvironment.Stop("redis-node-2")
	s.Require().NoError(err)

	defer func() {
		err = haDockerEnvironment.Start("redis-node-2")
		s.Require().NoError(err)
	}()

	s.doVisit(s.T(), s.Context(ctx), HomeBaseURL)
	s.verifyIsHome(s.T(), s.Context(ctx))

	// Verify the user is still authenticated.
	s.doVisit(s.T(), s.Context(ctx), GetLoginBaseURL())
	s.verifyIsSecondFactorPage(s.T(), s.Context(ctx))
}

func (s *HighAvailabilityWebDriverSuite) TestShouldKeepUserDataInDB() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer func() {
		cancel()
		s.collectScreenshot(ctx.Err(), s.Page)
	}()

	secret := s.doRegisterThenLogout(s.T(), s.Context(ctx), "john", "password")

	err := haDockerEnvironment.Restart("mariadb")
	s.Require().NoError(err)

	s.doLoginTwoFactor(s.T(), s.Context(ctx), "john", "password", false, secret, "")
	s.verifyIsSecondFactorPage(s.T(), s.Context(ctx))
}

func (s *HighAvailabilityWebDriverSuite) TestShouldKeepSessionAfterAutheliaRestart() {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer func() {
		cancel()
		s.collectScreenshot(ctx.Err(), s.Page)
	}()

	secret := s.doRegisterAndLogin2FA(s.T(), s.Context(ctx), "john", "password", false, "")
	s.verifyIsSecondFactorPage(s.T(), s.Context(ctx))

	err := haDockerEnvironment.Restart("authelia-backend")
	s.Require().NoError(err)

	err = waitUntilAutheliaBackendIsReady(haDockerEnvironment)
	s.Require().NoError(err)

	s.doVisit(s.T(), s.Context(ctx), HomeBaseURL)
	s.verifyIsHome(s.T(), s.Context(ctx))

	// Verify the user is still authenticated.
	s.doVisit(s.T(), s.Context(ctx), GetLoginBaseURL())
	s.verifyIsSecondFactorPage(s.T(), s.Context(ctx))

	// Then logout and login again to check the secret is still there.
	s.doLogout(s.T(), s.Context(ctx))
	s.verifyIsFirstFactorPage(s.T(), s.Context(ctx))

	s.doLoginTwoFactor(s.T(), s.Context(ctx), "john", "password", false, secret, fmt.Sprintf("%s/secret.html", SecureBaseURL))
	s.verifySecretAuthorized(s.T(), s.Context(ctx))
}

var UserJohn = "john"
var UserBob = "bob"
var UserHarry = "harry"

var Users = []string{UserJohn, UserBob, UserHarry}

var expectedAuthorizations = map[string](map[string]bool){
	fmt.Sprintf("%s/secret.html", PublicBaseURL): {
		UserJohn: true, UserBob: true, UserHarry: true,
	},
	fmt.Sprintf("%s/secret.html", SecureBaseURL): {
		UserJohn: true, UserBob: true, UserHarry: true,
	},
	fmt.Sprintf("%s/secret.html", AdminBaseURL): {
		UserJohn: true, UserBob: false, UserHarry: false,
	},
	fmt.Sprintf("%s/secret.html", SingleFactorBaseURL): {
		UserJohn: true, UserBob: true, UserHarry: true,
	},
	fmt.Sprintf("%s/secret.html", MX1MailBaseURL): {
		UserJohn: true, UserBob: true, UserHarry: false,
	},
	fmt.Sprintf("%s/secret.html", MX2MailBaseURL): {
		UserJohn: false, UserBob: true, UserHarry: false,
	},

	fmt.Sprintf("%s/groups/admin/secret.html", DevBaseURL): {
		UserJohn: true, UserBob: false, UserHarry: false,
	},
	fmt.Sprintf("%s/groups/dev/secret.html", DevBaseURL): {
		UserJohn: true, UserBob: true, UserHarry: false,
	},
	fmt.Sprintf("%s/users/john/secret.html", DevBaseURL): {
		UserJohn: true, UserBob: false, UserHarry: false,
	},
	fmt.Sprintf("%s/users/harry/secret.html", DevBaseURL): {
		UserJohn: true, UserBob: false, UserHarry: true,
	},
	fmt.Sprintf("%s/users/bob/secret.html", DevBaseURL): {
		UserJohn: true, UserBob: true, UserHarry: false,
	},
}

func (s *HighAvailabilityWebDriverSuite) TestShouldVerifyAccessControl() {
	verifyUserIsAuthorized := func(ctx context.Context, t *testing.T, targetURL string, authorized bool) {
		s.doVisit(t, s.Context(ctx), targetURL)
		s.verifyURLIs(t, s.Context(ctx), targetURL)

		if authorized {
			s.verifySecretAuthorized(t, s.Context(ctx))
		} else {
			s.verifyBodyContains(t, s.Context(ctx), "403 Forbidden")
		}
	}

	verifyAuthorization := func(username string) func(t *testing.T) {
		return func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer func() {
				s.collectScreenshot(ctx.Err(), s.Page)
				cancel()
			}()

			s.doRegisterAndLogin2FA(t, s.Context(ctx), username, "password", false, "")

			for url, authorizations := range expectedAuthorizations {
				t.Run(url, func(t *testing.T) {
					verifyUserIsAuthorized(ctx, t, url, authorizations[username])
				})
			}

			s.doLogout(t, s.Context(ctx))
		}
	}

	for _, user := range Users {
		s.T().Run(user, verifyAuthorization(user))
	}
}

type HighAvailabilitySuite struct {
	suite.Suite
}

func NewHighAvailabilitySuite() *HighAvailabilitySuite {
	return &HighAvailabilitySuite{}
}

func DoGetWithAuth(t *testing.T, username, password string) int {
	client := NewHTTPClient()
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/secret.html", SingleFactorBaseURL), nil)
	assert.NoError(t, err)
	req.SetBasicAuth(username, password)

	res, err := client.Do(req)
	assert.NoError(t, err)

	return res.StatusCode
}

func (s *HighAvailabilitySuite) TestBasicAuth() {
	s.Assert().Equal(DoGetWithAuth(s.T(), "john", "password"), 200)
	s.Assert().Equal(DoGetWithAuth(s.T(), "john", "bad-password"), 302)
	s.Assert().Equal(DoGetWithAuth(s.T(), "dontexist", "password"), 302)
}

func (s *HighAvailabilitySuite) TestOneFactorScenario() {
	suite.Run(s.T(), NewOneFactorScenario())
}

func (s *HighAvailabilitySuite) TestTwoFactorScenario() {
	suite.Run(s.T(), NewTwoFactorScenario())
}

func (s *HighAvailabilitySuite) TestRegulationScenario() {
	suite.Run(s.T(), NewRegulationScenario())
}

func (s *HighAvailabilitySuite) TestCustomHeadersScenario() {
	suite.Run(s.T(), NewCustomHeadersScenario())
}

func (s *HighAvailabilitySuite) TestRedirectionCheckScenario() {
	suite.Run(s.T(), NewRedirectionCheckScenario())
}

func (s *HighAvailabilitySuite) TestHighAvailabilityWebDriverSuite() {
	suite.Run(s.T(), NewHighAvailabilityWebDriverSuite())
}

func TestHighAvailabilityWebDriverSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping suite test in short mode")
	}

	suite.Run(t, NewHighAvailabilityWebDriverSuite())
}

func TestHighAvailabilitySuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping suite test in short mode")
	}

	suite.Run(t, NewHighAvailabilitySuite())
}
