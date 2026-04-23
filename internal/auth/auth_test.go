package auth_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"stronglifts/internal/auth"
	"stronglifts/internal/db"
	"stronglifts/internal/handlers"

	"github.com/go-chi/chi/v5"
)

type testTransport struct {
	server *httptest.Server
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.server.URL, "http://")
	return http.DefaultTransport.RoundTrip(req)
}

func newTestClient(server *httptest.Server) *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &testTransport{server: server},
	}
}

func newServerWithDB(t *testing.T, testDB *db.DB) *httptest.Server {
	t.Helper()

	sessionStore := auth.NewSessionStore(testDB.Conn())
	authHandlers := handlers.NewAuthHandlers(testDB, sessionStore)

	r := chi.NewRouter()
	r.Get("/register", authHandlers.Register)
	r.Post("/register", authHandlers.Register)
	r.Get("/login", authHandlers.Login)
	r.Post("/login", authHandlers.Login)
	r.Post("/logout", authHandlers.Logout)
	r.Group(func(r chi.Router) {
		r.Use(auth.SessionMiddleware(sessionStore))
		r.Get("/onboarding", authHandlers.Onboarding)
		r.Post("/onboarding", authHandlers.Onboarding)
		r.Get("/", authHandlers.Dashboard)
		r.Get("/profile", authHandlers.Profile)
		r.Post("/profile", authHandlers.Profile)
		r.Post("/account/delete", authHandlers.DeleteAccount)
	})

	return httptest.NewServer(r)
}

func newTestApp(t *testing.T) (*http.Client, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp db: %v", err)
	}
	tmpFile.Close()

	testDB, err := db.New(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	if err := testDB.CreateSchema(); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	server := newServerWithDB(t, testDB)
	client := newTestClient(server)

	cleanup := func() {
		server.Close()
		testDB.Close()
		os.Remove(tmpFile.Name())
	}

	return client, cleanup
}

func completeOnboarding(t *testing.T, client *http.Client, cookie string) {
	t.Helper()
	req, _ := http.NewRequest("POST", "http://app/onboarding", strings.NewReader("unit_pref=lb_in&squat=195&bench=135&row=95&press=95&deadlift=225"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", cookie)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /onboarding: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302 from onboarding, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRegisterPage(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, err := client.Get("http://app/register")
	if err != nil {
		t.Fatalf("GET /register: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Create account") {
		t.Error("page missing 'Create account'")
	}
}

func TestLoginPage(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, err := client.Get("http://app/login")
	if err != nil {
		t.Fatalf("GET /login: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Log in") {
		t.Error("page missing 'Log in'")
	}
}

func TestProtectedRouteRedirects(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, err := client.Get("http://app/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/login" {
		t.Errorf("expected redirect to /login, got %s", resp.Header.Get("Location"))
	}
}

func TestDashboardRedirectsToOnboardingBeforeSetup(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	cookie := resp.Header.Get("Set-Cookie")
	resp.Body.Close()

	req, _ := http.NewRequest("GET", "http://app/", nil)
	parts := strings.SplitN(cookie, ";", 2)
	req.Header.Set("Cookie", parts[0])
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET / before onboarding: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/onboarding" {
		t.Fatalf("expected redirect to /onboarding, got %s", resp.Header.Get("Location"))
	}
}

func TestOnboardingSeedsStartingWeights(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	cookie := resp.Header.Get("Set-Cookie")
	resp.Body.Close()

	parts := strings.SplitN(cookie, ";", 2)
	req, _ := http.NewRequest("POST", "http://app/onboarding", strings.NewReader("unit_pref=lb_in&squat=225&bench=155&row=115&press=105&deadlift=275"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", parts[0])
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /onboarding: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/workouts" {
		t.Fatalf("expected redirect to /workouts, got %s", resp.Header.Get("Location"))
	}
	resp.Body.Close()

	req, _ = http.NewRequest("GET", "http://app/", nil)
	req.Header.Set("Cookie", parts[0])
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET / after onboarding: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "225.0 lb") {
		t.Fatalf("expected seeded squat weight in dashboard, body: %s", body)
	}
}

func TestRegisterWithValidData(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, err := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	if err != nil {
		t.Fatalf("POST /register: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/login" {
		t.Errorf("expected redirect to /login, got %s", resp.Header.Get("Location"))
	}
}

func TestRegisterPreservesUsernameCase(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, err := client.PostForm("http://app/register", url.Values{
		"username": {"Alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	if err != nil {
		t.Fatalf("POST /register: %v", err)
	}
	resp.Body.Close()

	resp, err = client.PostForm("http://app/login", url.Values{
		"username": {"Alice"},
		"password": {"password123"},
	})
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	cookie := resp.Header.Get("Set-Cookie")
	if cookie == "" {
		t.Fatal("expected session cookie")
	}
	req, _ := http.NewRequest("GET", "http://app/profile", nil)
	parts := strings.SplitN(cookie, ";", 2)
	req.Header.Set("Cookie", parts[0])

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET /profile: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(body), "@Alice") {
		t.Fatalf("expected preserved-case username in profile, body: %s", body)
	}
}

func TestRegisterDuplicateUsernameCaseSensitive(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"Alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/register", url.Values{
		"username": {"aLiCe"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	resp.Body.Close()
	if resp.Header.Get("Location") != "/login" {
		t.Fatalf("expected redirect to /login, got %s", resp.Header.Get("Location"))
	}
}

func TestRegisterShortUsername(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"ab"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "at least 3 characters") {
		t.Error("missing username length error")
	}
}

func TestRegisterMismatchedPasswords(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"different"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "do not match") {
		t.Error("missing password mismatch error")
	}
}

func TestRegisterDuplicateUsername(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	form := url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	}
	resp, _ := client.PostForm("http://app/register", form)
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/register", form)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "already taken") {
		t.Error("missing duplicate username error")
	}
}

func TestLoginWithValidCredentials(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, err := client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/workouts" {
		t.Errorf("expected redirect to /workouts, got %s", resp.Header.Get("Location"))
	}
}

func TestLoginWithWrongPassword(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"wrongpassword"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Invalid username or password") {
		t.Error("missing invalid credentials error")
	}
}

func TestLoginUsernameIsCaseSensitive(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"Alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, err := client.PostForm("http://app/login", url.Values{
		"username": {"ALICE"},
		"password": {"password123"},
	})
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Invalid username or password") {
		t.Fatalf("expected invalid credentials error, body: %s", body)
	}
}

func TestAuthenticatedUserCanAccessDashboard(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	// Register and login
	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	cookie := resp.Header.Get("Set-Cookie")
	resp.Body.Close()
	parts := strings.SplitN(cookie, ";", 2)
	completeOnboarding(t, client, parts[0])

	// Access dashboard with cookie
	req, _ := http.NewRequest("GET", "http://app/", nil)
	if cookie != "" {
		// Extract just the cookie value
		req.Header.Set("Cookie", parts[0])
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Next Workout") {
		t.Error("dashboard missing workout content")
	}
}

func TestLogout(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	// Register and login
	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	resp.Body.Close()

	// Logout
	resp, err := client.Post("http://app/logout", "", nil)
	if err != nil {
		t.Fatalf("POST /logout: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/login" {
		t.Errorf("expected redirect to /login, got %s", resp.Header.Get("Location"))
	}
}

func TestDeleteAccountSoftDeletesAndBlocksLogin(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	cookie := resp.Header.Get("Set-Cookie")
	resp.Body.Close()

	req, _ := http.NewRequest("POST", "http://app/account/delete", nil)
	parts := strings.SplitN(cookie, ";", 2)
	req.Header.Set("Cookie", parts[0])
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /account/delete: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/login" {
		t.Fatalf("expected redirect to /login, got %s", resp.Header.Get("Location"))
	}
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 login form on failed login, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Invalid username or password") {
		t.Fatalf("expected invalid credentials message, got %s", body)
	}
}

func TestDeleteAccountInvalidatesCurrentSession(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	cookie := resp.Header.Get("Set-Cookie")
	resp.Body.Close()

	req, _ := http.NewRequest("POST", "http://app/account/delete", nil)
	parts := strings.SplitN(cookie, ";", 2)
	req.Header.Set("Cookie", parts[0])
	resp, _ = client.Do(req)
	resp.Body.Close()

	req, _ = http.NewRequest("GET", "http://app/", nil)
	req.Header.Set("Cookie", parts[0])
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET / with old cookie: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/login" {
		t.Fatalf("expected redirect to /login, got %s", resp.Header.Get("Location"))
	}
}

func TestProfilePageLoadsForAuthenticatedUser(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	cookie := resp.Header.Get("Set-Cookie")
	resp.Body.Close()

	req, _ := http.NewRequest("GET", "http://app/profile", nil)
	parts := strings.SplitN(cookie, ";", 2)
	req.Header.Set("Cookie", parts[0])
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /profile: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Profile & Settings") {
		t.Fatalf("profile page missing title, body: %s", body)
	}
	if !strings.Contains(string(body), "@alice") {
		t.Fatalf("profile page missing username, body: %s", body)
	}
	if !strings.Contains(string(body), "kg / cm") {
		t.Fatalf("profile page missing unit options, body: %s", body)
	}
}

func TestUpdateUnitPreferenceToImperial(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	cookie := resp.Header.Get("Set-Cookie")
	resp.Body.Close()

	req, _ := http.NewRequest("POST", "http://app/profile", strings.NewReader("unit_pref=lb_in"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	parts := strings.SplitN(cookie, ";", 2)
	req.Header.Set("Cookie", parts[0])
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /profile: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/profile?settings_saved=1" {
		t.Fatalf("expected redirect to /profile?settings_saved=1, got %s", resp.Header.Get("Location"))
	}
	resp.Body.Close()

	req, _ = http.NewRequest("GET", "http://app/profile", nil)
	req.Header.Set("Cookie", parts[0])
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET /profile after update: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), `value="lb_in" checked`) {
		t.Fatalf("expected imperial preference to be checked, body: %s", body)
	}
}

func TestProfileRejectsInvalidUnitPreference(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	cookie := resp.Header.Get("Set-Cookie")
	resp.Body.Close()

	req, _ := http.NewRequest("POST", "http://app/profile", strings.NewReader("unit_pref=invalid"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	parts := strings.SplitN(cookie, ";", 2)
	req.Header.Set("Cookie", parts[0])
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /profile invalid: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Invalid unit preference") {
		t.Fatalf("expected invalid preference error, body: %s", body)
	}
}

func TestProfileUpdatesPassword(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	cookie := resp.Header.Get("Set-Cookie")
	resp.Body.Close()

	req, _ := http.NewRequest("POST", "http://app/profile", strings.NewReader("action=password&current_password=password123&new_password=newpassword123&confirm_password=newpassword123"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	parts := strings.SplitN(cookie, ";", 2)
	req.Header.Set("Cookie", parts[0])
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /profile password: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/profile?password_saved=1" {
		t.Fatalf("expected redirect to /profile?password_saved=1, got %s", resp.Header.Get("Location"))
	}
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected old password login to fail with 200 form, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"newpassword123"},
	})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected new password login redirect, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestProfileRejectsWrongCurrentPassword(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	cookie := resp.Header.Get("Set-Cookie")
	resp.Body.Close()

	req, _ := http.NewRequest("POST", "http://app/profile", strings.NewReader("action=password&current_password=wrongpassword&new_password=newpassword123&confirm_password=newpassword123"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	parts := strings.SplitN(cookie, ";", 2)
	req.Header.Set("Cookie", parts[0])
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /profile wrong current password: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Current password is incorrect") {
		t.Fatalf("expected current password error, body: %s", body)
	}
}

func TestProfileRejectsMismatchedNewPasswords(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	cookie := resp.Header.Get("Set-Cookie")
	resp.Body.Close()

	req, _ := http.NewRequest("POST", "http://app/profile", strings.NewReader("action=password&current_password=password123&new_password=newpassword123&confirm_password=different123"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	parts := strings.SplitN(cookie, ";", 2)
	req.Header.Set("Cookie", parts[0])
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /profile mismatched new passwords: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "New passwords do not match") {
		t.Fatalf("expected mismatch error, body: %s", body)
	}
}

func TestProfileRejectsUnchangedPassword(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	cookie := resp.Header.Get("Set-Cookie")
	resp.Body.Close()

	req, _ := http.NewRequest("POST", "http://app/profile", strings.NewReader("action=password&current_password=password123&new_password=password123&confirm_password=password123"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	parts := strings.SplitN(cookie, ";", 2)
	req.Header.Set("Cookie", parts[0])
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /profile unchanged password: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "New password must be different from current password") {
		t.Fatalf("expected unchanged password error, body: %s", body)
	}
}

func TestDeleteAccountRequiresAuthentication(t *testing.T) {
	client, cleanup := newTestApp(t)
	defer cleanup()

	req, _ := http.NewRequest("POST", "http://app/account/delete", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /account/delete unauthenticated: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/login" {
		t.Fatalf("expected redirect to /login, got %s", resp.Header.Get("Location"))
	}
}

func TestSessionPersistsAcrossServerRestart(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "session_restart_*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	testDB, err := db.New(tmpFile.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer testDB.Close()

	if err := testDB.CreateSchema(); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	server := newServerWithDB(t, testDB)
	client := newTestClient(server)

	resp, _ := client.PostForm("http://app/register", url.Values{
		"username": {"alice"},
		"password": {"password123"},
		"confirm":  {"password123"},
	})
	resp.Body.Close()

	resp, _ = client.PostForm("http://app/login", url.Values{
		"username": {"alice"},
		"password": {"password123"},
	})
	cookie := resp.Header.Get("Set-Cookie")
	resp.Body.Close()
	parts := strings.SplitN(cookie, ";", 2)
	completeOnboarding(t, client, parts[0])
	server.Close()

	server = newServerWithDB(t, testDB)
	defer server.Close()
	client = newTestClient(server)

	req, _ := http.NewRequest("GET", "http://app/", nil)
	req.Header.Set("Cookie", parts[0])
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET / after restart: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after restart, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Next Workout") {
		t.Fatalf("expected dashboard after restart, body: %s", body)
	}
}
