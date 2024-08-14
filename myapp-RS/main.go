package main

import (
	_ "context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	_ "errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	_ "net/smtp"
	"os"
	_ "strconv"
	"strings"
	_ "sync"
	"time"

	"github.com/gorilla/sessions"
	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	"gopkg.in/mail.v2"
)

// type Message struct {
//     Sender   string `json:"sender"`
//     Receiver string `json:"receiver"`
//     Content  string `json:"content"`
// }

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var messageHistory = make(map[string][]Message)

type Message struct {
	Sender    string    `json:"sender"`
	Recipient string    `json:"recipient"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type Response struct {
	Messages []Message `json:"messages"`
}

type Client struct {
	conn  *websocket.Conn
	email string
}

var clients = make(map[*Client]bool)
var broadcast = make(chan Message)

// var clients = ClientManager{
//     clients: make(map[string]*Client),
// }

// var clients = struct {
//     sync.Mutex
//     clients map[string]*Client
// }{
//     clients: make(map[string]*Client),
// }

// type Messages struct {
//     Content string `json:"content"`
//     Date    string `json:"date"`
// }

// type Message struct {
//     ID       int       `json:"id"`
//     Sender   string    `json:"sender"`
//     Receiver string    `json:"receiver"`
//     Content  string    `json:"content"`
//     Date     time.Time `json:"date"`
//     Replied bool
// }

type Candidate struct {
	Name  string
	Email string
}

type Reply struct {
	ID       int
	Sender   string
	Receiver string
	Content  string
	Date     time.Time
}

// var store = sessions.NewCookieStore([]byte("your-secret-key"))

var store = sessions.NewCookieStore([]byte(os.Getenv("SESSION_SECRET")))

type Job struct {
	ID                  int
	JobTitle            string
	Company             string
	Location            string
	Salary              string
	JobDescription      string
	ApplicationDeadline string
	RecruiterEmail      string
}
type Applicant struct {
	JobID int
	Name  string
	Email string
	Phone string
}

type Application struct {
	JobID           int
	JobTitle        string
	RecruiterName   string
	ApplicationDate string
}
type LoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type PageData struct {
	Name          string
	Email         string
	Dob           string
	Qualification string
	GitHub        string
	Mobile        string
	ErrorMessage  string
}

// type Message struct {
//     ID        int
//     Sender    string
//     Receiver  string
//     Content   string
//     Timestamp time.Time
// }

var db *sql.DB
var templates *template.Template

func init() {
	var err error
	connStr := "user=postgres password=new_password dbname=candidate_db sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	//	_, err = db.Exec(`
	//	CREATE TABLE IF NOT EXISTS messages (
	//	    id SERIAL PRIMARY KEY,
	//	    sender TEXT NOT NULL,
	//	    recipient TEXT NOT NULL,
	//	    message TEXT NOT NULL,
	//	    timestamp TIMESTAMPTZ DEFAULT NOW()
	//	)
	//
	// `)
	if err != nil {
		log.Fatalf("Could not ensure table existence: %v", err)
	}

	templates = template.Must(template.ParseFiles(
		"static/admin_dashboard.html",
		"static/view_candidates.html",
		"static/view_recruiters.html",
		"static/candidate_dashboard.html",
		"static/recruiter_dashboard.html",
		"static/update_recruiter.html",
		"static/submitjob.html",
		"static/myjobs.html",
		"static/editjob.html",
		"static/viewapplications.html",
		"static/apply_for_job.html",
		"static/apply.html",
		"static/view_applied_jobs.html",
		"static/edit_profile.html",
		"static/view_applications.html",
		"static/connect.html",
		"static/candidates.html",
		"static/chat.html",
	))

}

func main() {
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/index.html")
	})

	http.HandleFunc("/set-session", setSessionHandler)
	http.HandleFunc("/get-session", getSessionHandler)

	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/admin_dashboard", adminDashboardHandler)
	http.HandleFunc("/view_candidates", viewCandidatesHandler)
	http.HandleFunc("/register_recruiter", registerRecruiterHandler)
	http.HandleFunc("/submit_recruiter_registration", submitRecruiterRegistrationHandler)
	http.HandleFunc("/candidate_dashboard", candidateDashboardHandler)
	http.HandleFunc("/view_recruiters", viewRecruitersHandler)
	http.HandleFunc("/update_recruiter", updateRecruiterHandler)
	http.HandleFunc("/submit_update_recruiter", submitUpdateRecruiterHandler)
	http.HandleFunc("/delete_recruiter", deleteRecruiterHandler)
	http.HandleFunc("/recruiter_dashboard", recruiterDashboardHandler)
	http.HandleFunc("/addjob", submitJobHandler)
	http.HandleFunc("/myjobs", viewMyJobsHandler)
	http.HandleFunc("/editjob/", editJobHandler)
	http.HandleFunc("/deletejob/", deleteJobHandler)
	http.HandleFunc("/apply/", applyJobHandler)
	http.HandleFunc("/submit_application", submitApplicationHandler)
	http.HandleFunc("/viewapplications", viewAllApplicationsHandler)
	http.HandleFunc("/view_applied_jobs", viewAppliedJobsHandler)
	http.HandleFunc("/edit_profile", editProfileHandler)
	http.HandleFunc("/update_profile", updateProfileHandler)
	http.HandleFunc("/view_applications", viewApplicationsHandler)
	http.HandleFunc("/delete_profile", deleteProfileHandler)
	http.HandleFunc("/send_reply", sendReplyHandler)

	http.HandleFunc("/user-email", userEmailHandler)
	http.HandleFunc("/api/check-auth", checkAuthHandler)
	http.HandleFunc("/fetch-chat-history", fetchChatHistoryHandler)
	http.HandleFunc("/get-user-email", getUserEmailHandler)
	http.HandleFunc("/ws", handleWebSocketConnection)
	go handleBroadcastMessages()

	// r := mux.NewRouter()
	http.HandleFunc("/get-messages", getMessagesHandler)
	http.HandleFunc("/ask", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/ask.html")
	})

	http.HandleFunc("/role", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			role := r.FormValue("role")
			switch role {
			case "candidate":
				http.Redirect(w, r, "/register_candidate", http.StatusSeeOther)
			case "recruiter":
				http.Redirect(w, r, "/register_recruiter", http.StatusSeeOther)
			default:
				http.Error(w, "Invalid role", http.StatusBadRequest)
			}
		} else {
			http.ServeFile(w, r, "./static/ask.html")
		}
	})

	http.HandleFunc("/register_candidate", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/register_candidate.html")
	})

	http.HandleFunc("/submit_candidate_registration", submitCandidateRegistrationHandler)

	http.HandleFunc("/candidates", candidatesHandler)

	// http.HandleFunc("/chat", handleConnection)
	// http.HandleFunc("/chat", handleWebSocket)
	http.HandleFunc("/chat", handleConnections)
	http.HandleFunc("/save_message", saveMessageHandler)

	http.HandleFunc("/save-messages", saveMessagesHandler)

	// go handleMessages()

	// http.HandleFunc("/save-message", saveMessageHandler)

	http.HandleFunc("/get_previous_messages", handleGetPreviousMessages)
	// go handleMessages()
	http.HandleFunc("/connect", connectHandler)

	// http.HandleFunc("/send_message", sendMessageHandler)
	// http.HandleFunc("/view_messages", viewMessagesHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// http.HandleFunc("/ws", handleConnection)
	// http.HandleFunc("/chat", chatHandler)

	log.Printf("Server started at http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func setSessionHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session-name")
	session.Values["foo"] = "bar"
	session.Save(r, w)
	fmt.Fprintln(w, "Session value set")
}

func getSessionHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session-name")
	foo := session.Values["foo"]
	fmt.Fprintf(w, "Session value: %s", foo)
}

func userEmailHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("username")
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	email := cookie.Value
	response := map[string]string{"email": email}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")

		log.Printf("Login attempt for username: %s", username)

		var dbPassword, dbName, dbEmail, dbRole string
		err := db.QueryRow(`
            SELECT name, email, password, 'candidate' AS role FROM candidate11 WHERE email=$1
            UNION
            SELECT name, email, password, 'recruiter' AS role FROM recruiter WHERE email=$1
            UNION
            SELECT name, email, password, 'admin' AS role FROM admin WHERE email=$1
        `, username).Scan(&dbName, &dbEmail, &dbPassword, &dbRole)

		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("No user found with email: %s", username)
				http.Redirect(w, r, "/login?error=1", http.StatusSeeOther)
			} else {
				log.Printf("Error querying database: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		if dbPassword == password {
			http.SetCookie(w, &http.Cookie{
				Name:     "username",
				Value:    dbEmail,
				Path:     "/",
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteLaxMode,
			})

			switch dbRole {
			case "candidate":
				log.Printf("Redirecting to candidate dashboard")
				http.Redirect(w, r, "/candidate_dashboard", http.StatusSeeOther)
			case "recruiter":
				log.Printf("Redirecting to recruiter dashboard")
				http.Redirect(w, r, "/recruiter_dashboard", http.StatusSeeOther)
			case "admin":
				log.Printf("Redirecting to admin dashboard")
				http.Redirect(w, r, "/admin_dashboard", http.StatusSeeOther)
			default:
				log.Printf("Invalid role: %s", dbRole)
				http.Error(w, "Invalid role", http.StatusUnauthorized)
			}
			return
		} else {
			log.Printf("Password mismatch for user: %s", username)
			http.Redirect(w, r, "/login?error=1", http.StatusSeeOther)
		}
	} else {
		http.ServeFile(w, r, "./static/login.html")
	}
}

func adminDashboardHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("username")
	if err != nil {
		if err == http.ErrNoCookie {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		log.Println("Error retrieving cookie:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	username := cookie.Value
	var name, email, role string

	err = db.QueryRow("SELECT name, email, 'admin' AS role FROM admin WHERE email=$1", username).Scan(&name, &email, &role)
	if err != nil {
		log.Println("Error querying database:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := struct {
		Name  string
		Email string
		Role  string
	}{
		Name:  name,
		Email: email,
		Role:  role,
	}

	templates.ExecuteTemplate(w, "admin_dashboard.html", data)
}

func viewCandidatesHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
        SELECT a.name AS candidate_name, a.email, a.phone AS mobile, j.job_title AS job_title, a.application_date, a.resume
        FROM applications a
        JOIN jobs j ON a.job_id = j.id
    `)
	if err != nil {
		log.Println("Error querying database:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var candidates []struct {
		Name            string
		Email           string
		Mobile          string
		JobTitle        string
		ApplicationDate string
		ResumeURL       string
	}
	for rows.Next() {
		var c struct {
			Name            string
			Email           string
			Mobile          string
			JobTitle        string
			ApplicationDate string
			ResumeURL       string
		}
		err := rows.Scan(&c.Name, &c.Email, &c.Mobile, &c.JobTitle, &c.ApplicationDate, &c.ResumeURL)
		if err != nil {
			log.Println("Error scanning row:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		candidates = append(candidates, c)
	}

	data := struct {
		Candidates []struct {
			Name            string
			Email           string
			Mobile          string
			JobTitle        string
			ApplicationDate string
			ResumeURL       string
		}
	}{
		Candidates: candidates,
	}

	err = templates.ExecuteTemplate(w, "view_candidates.html", data)
	if err != nil {
		log.Println("Error rendering template:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func viewRecruitersHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("viewRecruitersHandler called")

	cookie, err := r.Cookie("username")
	if err != nil {
		if err == http.ErrNoCookie {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		log.Println("Error retrieving cookie:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Println("User authenticated:", cookie.Value)

	rows, err := db.Query("SELECT name, email, mobile, post, company, experience, branch FROM recruiter")
	if err != nil {
		log.Println("Error querying database:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Recruiter struct {
		Name       string
		Email      string
		Mobile     string
		Post       string
		Company    string
		Experience string
		Branch     string
	}

	var recruiters []Recruiter
	for rows.Next() {
		var r Recruiter
		err := rows.Scan(&r.Name, &r.Email, &r.Mobile, &r.Post, &r.Company, &r.Experience, &r.Branch)
		if err != nil {
			log.Println("Error scanning row:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		recruiters = append(recruiters, r)
	}

	log.Printf("Found %d recruiters\n", len(recruiters))

	data := struct {
		Recruiters []Recruiter
	}{
		Recruiters: recruiters,
	}

	err = templates.ExecuteTemplate(w, "view_recruiters.html", data)
	if err != nil {
		log.Println("Error executing template:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Println("viewRecruitersHandler completed successfully")
}

func registerRecruiterHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./static/register_recruiter.html")
}

func submitRecruiterRegistrationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		email := r.FormValue("email")
		password := r.FormValue("password")
		post := r.FormValue("post")
		company := r.FormValue("company")
		experience := r.FormValue("experience")
		branch := r.FormValue("branch")
		mobile := r.FormValue("mobile")

		_, err := db.Exec(`
            INSERT INTO recruiter (name, email, password, post, company, experience, branch, mobile)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			name, email, password, post, company, experience, branch, mobile)

		if err != nil {
			log.Println("Error inserting data:", err)
			http.Error(w, "Unable to save data", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/admin_dashboard", http.StatusSeeOther)
	} else {
		http.ServeFile(w, r, "./static/register_recruiter.html")
	}
}

func submitCandidateRegistrationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		email := r.FormValue("email")
		password := r.FormValue("password")
		dob := r.FormValue("dob")
		qualification := r.FormValue("qualification")
		github := r.FormValue("github")
		mobile := r.FormValue("mobile")

		var existingEmail string
		err := db.QueryRow("SELECT email FROM candidate11 WHERE email = $1", email).Scan(&existingEmail)
		if err == nil {
			renderPage(w, "register_candidate.html", PageData{ErrorMessage: "Email is already registered. Please try logging in."})
			return
		} else if err != sql.ErrNoRows {
			log.Println("Error checking existing email:", err)
			renderPage(w, "register_candidate.html", PageData{ErrorMessage: "Internal server error"})
			return
		}

		_, err = db.Exec(`
            INSERT INTO candidate11 (name, email, password, dob, qualification, github, mobile)
            VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			name, email, password, dob, qualification, github, mobile)
		if err != nil {
			log.Println("Error inserting data:", err)
			renderPage(w, "register_candidate.html", PageData{ErrorMessage: "Unable to save data"})
			return
		}

		if err := sendEmail(name, email, password, dob, qualification, github, mobile); err != nil {
			log.Println("Error sending email:", err)
			renderPage(w, "register_candidate.html", PageData{ErrorMessage: "Unable to send email"})
			return
		}

		http.Redirect(w, r, "/login", http.StatusSeeOther)
	} else {
		renderPage(w, "register_candidate.html", PageData{})
	}
}

func deleteProfileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		email := r.FormValue("email")

		tx, err := db.Begin()
		if err != nil {
			log.Println("Error starting transaction:", err)
			renderPage(w, "candidate_dashboard.html", PageData{ErrorMessage: "Internal server error"})
			return
		}

		defer tx.Rollback()

		_, err = tx.Exec("DELETE FROM candidate11 WHERE email = $1", email)
		if err != nil {
			log.Println("Error deleting from candidate11:", err)
			renderPage(w, "candidate_dashboard.html", PageData{ErrorMessage: "Unable to delete profile"})
			return
		}

		err = tx.Commit()
		if err != nil {
			log.Println("Error committing transaction:", err)
			renderPage(w, "candidate_dashboard.html", PageData{ErrorMessage: "Internal server error"})
			return
		}

		http.Redirect(w, r, "/", http.StatusSeeOther)
	} else {
		http.ServeFile(w, r, "./static/candidate_dashboard.html")
	}
}

func renderPage(w http.ResponseWriter, tmpl string, data PageData) {
	t, err := template.ParseFiles("./static/" + tmpl)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	t.Execute(w, data)
}

func sendEmail(name, email, password, dob, qualification, github, mobile string) error {
	m := mail.NewMessage()
	m.SetHeader("From", "subhashreelenka56@gmail.com")
	m.SetHeader("To", email)
	m.SetHeader("Subject", "Candidate Registration Details")
	m.SetBody("text/plain", "Dear "+name+",\n\n"+
		"Thank you for registering. Here are your details:\n"+
		"Name: "+name+"\n"+
		"Email: "+email+"\n"+
		"Password: "+password+"\n"+
		"Date of Birth: "+dob+"\n"+
		"Qualification: "+qualification+"\n"+
		"GitHub: "+github+"\n"+
		"Mobile: "+mobile+"\n\n"+
		"Best regards,\nCarrerConnect")

	d := mail.NewDialer("smtp.gmail.com", 587, "subhashreelenka56@gmail.com", "whmcysidkdintfls")
	d.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	return d.DialAndSend(m)
}

func candidateDashboardHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("username")
	if err != nil {
		if err == http.ErrNoCookie {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		log.Println("Error retrieving cookie:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	username := cookie.Value
	var name, email, dob, qualification, github, mobile string

	err = db.QueryRow("SELECT name, email, dob, qualification, github, mobile FROM candidate11 WHERE email=$1", username).Scan(&name, &email, &dob, &qualification, &github, &mobile)
	if err != nil {
		log.Println("Error querying candidate details:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := struct {
		Name          string
		Email         string
		Dob           string
		Qualification string
		GitHub        string
		Mobile        string
	}{
		Name:          name,
		Email:         email,
		Dob:           dob,
		Qualification: qualification,
		GitHub:        github,
		Mobile:        mobile,
	}

	templates.ExecuteTemplate(w, "candidate_dashboard.html", data)
}

func recruiterDashboardHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("username")
	if err != nil {
		if err == http.ErrNoCookie {
			log.Println("something")
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		log.Println("Error retrieving cookie:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	email := cookie.Value

	var name, post, company, branch, mobile, experience string
	err = db.QueryRow("SELECT name, email, post, company, branch, mobile, experience FROM recruiter WHERE email=$1", email).Scan(&name, &email, &post, &company, &branch, &mobile, &experience)
	if err != nil {
		log.Println("Error querying recruiter details:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// messages, err := FetchMessages(email)
	// if err != nil {
	//     log.Println("Error querying messages:", err)
	//     http.Error(w, "Internal server error", http.StatusInternalServerError)
	//     return
	// }

	data := struct {
		Name       string
		Email      string
		Post       string
		Company    string
		Branch     string
		Mobile     string
		Experience string
		Messages   []Message
	}{
		Name:       name,
		Email:      email,
		Post:       post,
		Company:    company,
		Branch:     branch,
		Mobile:     mobile,
		Experience: experience,
		// Messages:   messages,
	}

	tmpl := template.Must(template.ParseFiles("static/recruiter_dashboard.html"))
	tmpl.Execute(w, data)
}

func updateRecruiterHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request: %s %s", r.Method, r.URL.String())

	email := r.URL.Query().Get("email")
	log.Printf("Received email parameter: %s", email)

	if email == "" {
		log.Println("Email parameter is missing in the request")
		http.Error(w, "Email parameter is missing", http.StatusBadRequest)
		return
	}

	var name, post, company, experience, branch, mobile string

	log.Printf("Querying database for email: %s", email)
	err := db.QueryRow("SELECT name, post, company, experience, branch, mobile FROM recruiter WHERE email=$1", email).Scan(&name, &post, &company, &experience, &branch, &mobile)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Recruiter not found", http.StatusNotFound)
		} else {
			log.Printf("Error querying recruiter details: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	data := struct {
		Name       string
		Email      string
		Post       string
		Company    string
		Experience string
		Branch     string
		Mobile     string
	}{
		Name:       name,
		Email:      email,
		Post:       post,
		Company:    company,
		Experience: experience,
		Branch:     branch,
		Mobile:     mobile,
	}

	err = templates.ExecuteTemplate(w, "update_recruiter.html", data)
	if err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func submitUpdateRecruiterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	email := r.FormValue("email")
	name := r.FormValue("name")
	post := r.FormValue("post")
	company := r.FormValue("company")
	experience := r.FormValue("experience")
	branch := r.FormValue("branch")
	mobile := r.FormValue("mobile")

	log.Printf("Updating recruiter: email=%s, name=%s, post=%s, company=%s, experience=%s, branch=%s, mobile=%s",
		email, name, post, company, experience, branch, mobile)

	_, err := db.Exec(`UPDATE recruiter 
                        SET name=$1, post=$2, company=$3, experience=$4, branch=$5, mobile=$6 
                        WHERE email=$7`,
		name, post, company, experience, branch, mobile, email)

	if err != nil {
		log.Printf("Error updating recruiter details: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/view_recruiters", http.StatusSeeOther)
}

func deleteRecruiterHandler(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	_, err := db.Exec("DELETE FROM recruiter WHERE email=$1", email)
	if err != nil {
		log.Println("Error deleting recruiter:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/view_recruiters", http.StatusSeeOther)
}

func submitJobHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		log.Println("Received GET request for submit job page")
		cookie, err := r.Cookie("username")
		if err != nil {
			if err == http.ErrNoCookie {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			log.Println("Error retrieving cookie:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		email := cookie.Value
		var role string
		err = db.QueryRow("SELECT 'recruiter' AS role FROM recruiter WHERE email=$1", email).Scan(&role)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			log.Println("Error checking recruiter role:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		err = templates.ExecuteTemplate(w, "submitjob.html", nil)
		if err != nil {
			log.Println("Error rendering template:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}

	case http.MethodPost:
		log.Println("Received POST request to submit a job")
		cookie, err := r.Cookie("username")
		if err != nil {
			if err == http.ErrNoCookie {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			log.Println("Error retrieving cookie:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		email := cookie.Value
		var role string
		err = db.QueryRow("SELECT 'recruiter' AS role FROM recruiter WHERE email=$1", email).Scan(&role)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			log.Println("Error checking recruiter role:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		err = r.ParseForm()
		if err != nil {
			log.Println("Error parsing form:", err)
			http.Error(w, "Unable to parse form", http.StatusBadRequest)
			return
		}

		jobTitle := r.FormValue("jobTitle")
		company := r.FormValue("company")
		location := r.FormValue("location")
		salary := r.FormValue("salary")
		jobDescription := r.FormValue("jobDescription")
		applicationDeadline := r.FormValue("applicationDeadline")

		log.Printf("Form values: jobTitle=%s, company=%s, location=%s, salary=%s, jobDescription=%s, applicationDeadline=%s",
			jobTitle, company, location, salary, jobDescription, applicationDeadline)

		if jobTitle == "" || company == "" || location == "" || salary == "" || jobDescription == "" || applicationDeadline == "" {
			log.Println("Validation failed: All fields are required")
			http.Error(w, "All fields are required", http.StatusBadRequest)
			return
		}

		query := `INSERT INTO jobs (job_title, company, location, salary, job_description, application_deadline, recruiter_email) VALUES ($1, $2, $3, $4, $5, $6, $7)`
		_, err = db.Exec(query, jobTitle, company, location, salary, jobDescription, applicationDeadline, email)
		if err != nil {
			log.Println("Error inserting job:", err)
			http.Error(w, "Failed to add job", http.StatusInternalServerError)
			return
		}

		log.Println("Job added successfully")
		http.Redirect(w, r, "/recruiter_dashboard", http.StatusSeeOther)

	default:
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
	}
}

func viewMyJobsHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("username")
	if err != nil {
		if err == http.ErrNoCookie {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		log.Println("Error retrieving cookie:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	email := cookie.Value
	rows, err := db.Query(`
        SELECT id, job_title, company, location, salary, job_description, application_deadline
        FROM jobs
        WHERE recruiter_email = $1`, email)
	if err != nil {
		log.Println("Error retrieving jobs:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var job Job
		err = rows.Scan(&job.ID, &job.JobTitle, &job.Company, &job.Location, &job.Salary, &job.JobDescription, &job.ApplicationDeadline)
		if err != nil {
			log.Println("Error scanning job row:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		jobs = append(jobs, job)
	}

	if err = rows.Err(); err != nil {
		log.Println("Error iterating over job rows:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	err = templates.ExecuteTemplate(w, "myjobs.html", jobs)
	if err != nil {
		log.Println("Error rendering template:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func editJobHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/editjob/"):]

	cookie, err := r.Cookie("username")
	if err != nil {
		if err == http.ErrNoCookie {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		log.Println("Error retrieving cookie:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	email := cookie.Value

	switch r.Method {
	case http.MethodGet:
		var job Job
		err = db.QueryRow(`
            SELECT id, job_title, company, location, salary, job_description, application_deadline
            FROM jobs
            WHERE id = $1 AND recruiter_email = $2`, idStr, email).Scan(
			&job.ID, &job.JobTitle, &job.Company, &job.Location, &job.Salary, &job.JobDescription, &job.ApplicationDeadline)
		if err != nil {
			if err == sql.ErrNoRows {
				log.Println("Job not found with ID:", idStr, "for recruiter:", email)
				http.Error(w, "Job not found", http.StatusNotFound)
				return
			}
			log.Println("Error retrieving job:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		err = templates.ExecuteTemplate(w, "editjob.html", job)
		if err != nil {
			log.Println("Error rendering template:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}

	case http.MethodPost:
		err = r.ParseForm()
		if err != nil {
			log.Println("Error parsing form:", err)
			http.Error(w, "Unable to parse form", http.StatusBadRequest)
			return
		}

		jobTitle := r.FormValue("jobTitle")
		company := r.FormValue("company")
		location := r.FormValue("location")
		salary := r.FormValue("salary")
		jobDescription := r.FormValue("jobDescription")
		applicationDeadline := r.FormValue("applicationDeadline")

		log.Printf("Form values: jobTitle=%s, company=%s, location=%s, salary=%s, jobDescription=%s, applicationDeadline=%s",
			jobTitle, company, location, salary, jobDescription, applicationDeadline)

		query := `UPDATE jobs SET job_title = $1, company = $2, location = $3, salary = $4, job_description = $5, application_deadline = $6
                  WHERE id = $7 AND recruiter_email = $8`
		_, err = db.Exec(query, jobTitle, company, location, salary, jobDescription, applicationDeadline, idStr, email)
		if err != nil {
			log.Println("Error updating job:", err)
			http.Error(w, "Failed to update job", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/myjobs", http.StatusSeeOther)

	default:
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
	}
}

func deleteJobHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/deletejob/"):]

	cookie, err := r.Cookie("username")
	if err != nil {
		if err == http.ErrNoCookie {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		log.Println("Error retrieving cookie:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	email := cookie.Value

	_, err = db.Exec(`DELETE FROM jobs WHERE id = $1 AND recruiter_email = $2`, idStr, email)
	if err != nil {
		log.Println("Error deleting job:", err)
		http.Error(w, "Failed to delete job", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/myjobs", http.StatusSeeOther)
}

func viewAllApplicationsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
        SELECT a.job_id, j.job_title, r.name AS recruiter_name, a.application_date
        FROM applications a
        JOIN jobs j ON a.job_id = j.id
        JOIN recruiter r ON j.recruiter_email = r.email
        ORDER BY a.application_date DESC`)
	if err != nil {
		log.Printf("Error retrieving applications: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var applications []Application

	for rows.Next() {
		var app Application
		if err := rows.Scan(&app.JobID, &app.JobTitle, &app.RecruiterName, &app.ApplicationDate); err != nil {
			log.Printf("Error scanning application: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		applications = append(applications, app)
	}

	tmpl, err := template.ParseFiles("static/viewapplications.html")

	if err != nil {
		log.Printf("Error parsing template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, applications); err != nil {
		log.Printf("Error rendering template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func viewApplicationsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		log.Println("Received GET request for view applications page")

		_, err := r.Cookie("username")
		if err != nil {
			if err == http.ErrNoCookie {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			log.Println("Error retrieving cookie:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		rows, err := db.Query(`
            SELECT a.id AS application_id, j.job_title AS job_title, a.name AS candidate_name, a.email, a.phone AS mobile, a.application_date, a.resume, j.company AS company_name
            FROM applications a
            JOIN jobs j ON a.job_id = j.id
        `)
		if err != nil {
			log.Println("Error retrieving job applications:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		applications := []struct {
			ApplicationID   int
			JobTitle        string
			CandidateName   string
			Email           string
			Mobile          string
			ApplicationDate string
			ResumeURL       string
			CompanyName     string
		}{}

		for rows.Next() {
			var app struct {
				ApplicationID   int
				JobTitle        string
				CandidateName   string
				Email           string
				Mobile          string
				ApplicationDate string
				ResumeURL       string
				CompanyName     string
			}
			if err := rows.Scan(&app.ApplicationID, &app.JobTitle, &app.CandidateName, &app.Email, &app.Mobile, &app.ApplicationDate, &app.ResumeURL, &app.CompanyName); err != nil {
				log.Println("Error scanning row:", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			applications = append(applications, app)
		}

		if err := templates.ExecuteTemplate(w, "view_applications.html", applications); err != nil {
			log.Println("Error rendering template:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}

	case http.MethodPost:
		log.Println("Received POST request to update application status")

		applicationID := r.FormValue("applicationID")
		action := r.FormValue("action")

		if action != "accept" && action != "reject" {
			http.Error(w, "Invalid action", http.StatusBadRequest)
			return
		}

		err := updateApplicationStatus(applicationID, action)
		if err != nil {
			log.Println("Error updating application status:", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		log.Println("Application status updated successfully")
		http.Redirect(w, r, "/view_applications", http.StatusSeeOther)

	default:
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
	}
}

func updateApplicationStatus(applicationID, action string) error {
	var candidateEmail, candidateName, companyName, status string

	row := db.QueryRow(`
        SELECT a.email, a.name, j.company
        FROM applications a
        JOIN jobs j ON a.job_id = j.id
        WHERE a.id = $1`, applicationID)
	if err := row.Scan(&candidateEmail, &candidateName, &companyName); err != nil {
		log.Println("Error retrieving candidate details:", err)
		return err
	}

	var err error
	if action == "reject" {
		_, err = db.Exec(`DELETE FROM applications WHERE id = $1`, applicationID)
		status = "rejected"
	} else {
		_, err = db.Exec(`UPDATE applications SET status = $1 WHERE id = $2`, "accepted", applicationID)
		status = "accepted"
	}

	if err != nil {
		return err
	}

	return sendStatusEmail(candidateEmail, candidateName, status, companyName)
}

func sendStatusEmail(email, name, status, companyName string) error {
	m := mail.NewMessage()
	m.SetHeader("From", "subhashreelenka56@gmail.com")
	m.SetHeader("To", email)
	m.SetHeader("Subject", "Application Status Update")
	m.SetBody("text/plain", "Dear "+name+",\n\n"+
		"Your application has been  "+status+" by Our Team .\n\n"+
		"Company: "+companyName+"\n\n"+
		"Best regards,\nYour Company")

	d := mail.NewDialer("smtp.gmail.com", 587, "subhashreelenka56@gmail.com", "whmcysidkdintfls")
	d.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	return d.DialAndSend(m)
}

func applyJobHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	const prefix = "/apply/"
	if !strings.HasPrefix(path, prefix) {
		http.Error(w, "Invalid URL path", http.StatusBadRequest)
		return
	}
	jobID := strings.TrimPrefix(path, prefix)

	if jobID == "" {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}

	cookie, err := r.Cookie("username")
	if err != nil {
		if err == http.ErrNoCookie {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		log.Printf("Error retrieving cookie: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	email := cookie.Value

	var candidate struct {
		Name   string
		Email  string
		Mobile string
	}
	err = db.QueryRow(`
        SELECT name, email, mobile
        FROM candidate11 
        WHERE email = $1`, email).Scan(&candidate.Name, &candidate.Email, &candidate.Mobile)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Candidate not found", http.StatusNotFound)
			return
		}
		log.Printf("Error retrieving candidate details: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	var job struct {
		ID                  string
		JobTitle            string
		Company             string
		Location            string
		Salary              string
		ApplicationDeadline string
		JobDescription      string
	}
	err = db.QueryRow(`
        SELECT id, job_title, company, location, salary, application_deadline, job_description
        FROM jobs
        WHERE id = $1`, jobID).Scan(&job.ID, &job.JobTitle, &job.Company, &job.Location, &job.Salary, &job.ApplicationDeadline, &job.JobDescription)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Job not found", http.StatusNotFound)
			return
		}
		log.Printf("Error retrieving job details: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := struct {
		JobID  string
		Name   string
		Email  string
		Mobile string
	}{
		JobID:  jobID,
		Name:   candidate.Name,
		Email:  candidate.Email,
		Mobile: candidate.Mobile,
	}

	err = templates.ExecuteTemplate(w, "apply_for_job.html", data)
	if err != nil {
		log.Printf("Error rendering template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func submitApplicationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form: %v", err)
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	jobID := r.FormValue("jobID")
	name := r.FormValue("name")
	email := r.FormValue("email")
	mobile := r.FormValue("mobile")
	resumeURL := r.FormValue("resume")

	log.Printf("Form Values: jobID=%s, name=%s, email=%s, mobile=%s, resumeURL=%s", jobID, name, email, mobile, resumeURL)

	if jobID == "" || name == "" || email == "" || mobile == "" || resumeURL == "" {
		log.Println("Validation failed: All fields are required")
		http.Error(w, "All fields are required", http.StatusBadRequest)
		return
	}

	var existingApplicationCount int
	err := db.QueryRow(`
        SELECT COUNT(*)
        FROM applications
        WHERE job_id = $1 AND email = $2`, jobID, email).Scan(&existingApplicationCount)
	if err != nil {
		log.Printf("Error checking existing applications: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if existingApplicationCount > 0 {
		log.Println("Candidate has already applied for this job")
		w.Header().Set("Content-Type", "text/html")
		response := `
            <script>
                alert("You have already applied for this job.");
                window.history.back();
            </script>
        `
		w.Write([]byte(response))
		return
	}

	applicationDate := time.Now().Format(time.RFC3339)
	query := `
        INSERT INTO applications (job_id, name, email, phone, resume, application_date)
        VALUES ($1, $2, $3, $4, $5, $6)`
	_, err = db.Exec(query, jobID, name, email, mobile, resumeURL, applicationDate)
	if err != nil {
		log.Printf("Error submitting application: %v", err)
		http.Error(w, "Failed to submit application", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/viewapplications", http.StatusSeeOther)
}

func viewAppliedJobsHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("username")
	if err != nil {
		if err == http.ErrNoCookie {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		log.Printf("Error retrieving cookie: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	email := cookie.Value

	rows, err := db.Query(`
        SELECT a.job_id, j.job_title, j.company, j.location, j.salary, j.application_deadline, j.job_description
        FROM applications a
        JOIN jobs j ON a.job_id = j.id
        WHERE a.email = $1`, email)
	if err != nil {
		log.Printf("Error querying applications: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var applications []struct {
		JobID               string
		JobTitle            string
		Company             string
		Location            string
		Salary              string
		ApplicationDeadline string
		JobDescription      string
	}
	for rows.Next() {
		var app struct {
			JobID               string
			JobTitle            string
			Company             string
			Location            string
			Salary              string
			ApplicationDeadline string
			JobDescription      string
		}
		if err := rows.Scan(&app.JobID, &app.JobTitle, &app.Company, &app.Location, &app.Salary, &app.ApplicationDeadline, &app.JobDescription); err != nil {
			log.Printf("Error scanning row: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		applications = append(applications, app)
	}
	if err := rows.Err(); err != nil {
		log.Printf("Error iterating rows: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = templates.ExecuteTemplate(w, "view_applied_jobs.html", applications)
	if err != nil {
		log.Printf("Error rendering template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func editProfileHandler(w http.ResponseWriter, r *http.Request) {

	cookie, err := r.Cookie("username")
	if err != nil {
		if err == http.ErrNoCookie {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		log.Printf("Error retrieving cookie: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	email := cookie.Value

	var candidate struct {
		Name          string
		Email         string
		Dob           string
		Qualification string
		GitHub        string
		Mobile        string
	}
	err = db.QueryRow(`
        SELECT name, email, dob, qualification, github, mobile
        FROM candidate11
        WHERE email = $1`, email).Scan(&candidate.Name, &candidate.Email, &candidate.Dob, &candidate.Qualification, &candidate.GitHub, &candidate.Mobile)
	if err != nil {
		log.Printf("Error retrieving candidate details: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	err = templates.ExecuteTemplate(w, "edit_profile.html", candidate)
	if err != nil {
		log.Printf("Error rendering template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func updateProfileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form: %v", err)
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	name := r.FormValue("name")
	dob := r.FormValue("dob")
	qualification := r.FormValue("qualification")
	github := r.FormValue("github")
	mobile := r.FormValue("mobile")

	if name == "" || dob == "" || qualification == "" || github == "" || mobile == "" {
		log.Println("Validation failed: All fields are required")
		http.Error(w, "All fields are required", http.StatusBadRequest)
		return
	}

	query := `
        UPDATE candidate11
        SET name = $1, dob = $2, qualification = $3, github = $4, mobile = $5
        WHERE email = $6`
	_, err := db.Exec(query, name, dob, qualification, github, mobile, email)
	if err != nil {
		log.Printf("Error updating profile: %v", err)
		http.Error(w, "Failed to update profile", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/candidate_dashboard", http.StatusSeeOther)
}

// func connectHandler(w http.ResponseWriter, r *http.Request) {
//     email := r.URL.Query().Get("email")

//     fmt.Println("Received email parameter:", email) // Debugging line

//     if email == "" {
//         http.Error(w, "Email parameter is missing", http.StatusBadRequest)
//         return
//     }

//     err := templates.ExecuteTemplate(w, "connect.html", struct {
//         RecruiterEmail string
//     }{
//         RecruiterEmail: email,
//     })
//     if err != nil {
//         http.Error(w, "Unable to render connect page", http.StatusInternalServerError)
//     }
// }

func sendMessageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	receiver := r.FormValue("receiver")
	sender := r.FormValue("sender")
	content := r.FormValue("content")

	_, err := db.Exec("INSERT INTO messages (sender, receiver, content, timestamp) VALUES ($1, $2, $3, NOW())", sender, receiver, content)
	if err != nil {
		http.Error(w, "Failed to send message", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/view_recruiters", http.StatusSeeOther)
}

// func insertMessage(message Message) error {
//     query := `INSERT INTO messages (sender, receiver, content, date) VALUES (?, ?, ?, ?)`
//     _, err := db.Exec(query, message.Sender, message.Receiver, message.Content, message.Date)
//     return err
// }

// func viewMessagesHandler(w http.ResponseWriter, r *http.Request) {
//     email := r.URL.Query().Get("email")

//     if email == "" {
//         http.Error(w, "Email is required", http.StatusBadRequest)
//         return
//     }

//     rows, err := db.Query("SELECT sender, receiver, content, timestamp FROM messages WHERE sender = $1 OR receiver = $1 ORDER BY timestamp ASC", email)
//     if err != nil {
//         http.Error(w, "Unable to load messages", http.StatusInternalServerError)
//         return
//     }
//     defer rows.Close()

//     var messages []Message
//     for rows.Next() {
//         var message Message
//         if err := rows.Scan(&message.Sender, &message.Receiver, &message.Content, &message.Timestamp); err != nil {
//             http.Error(w, "Error scanning messages", http.StatusInternalServerError)
//             return
//         }
//         messages = append(messages, message)
//     }

//     tmpl := template.Must(template.ParseFiles("static/connect.html"))
//     if err := tmpl.Execute(w, struct {
//         Email    string
//         Messages []Message
//     }{
//         Email:    email,
//         Messages: messages,
//     }); err != nil {
//         http.Error(w, "Error executing template", http.StatusInternalServerError)
//     }
// }

func connectHandler(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")

	var name string
	err := db.QueryRow("SELECT name FROM recruiter WHERE email = $1", email).Scan(&name)
	if err != nil {
		http.Error(w, "Recruiter not found", http.StatusNotFound)
		return
	}

	tmpl, err := template.ParseFiles("static/connect.html")
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}

	data := struct {
		Name  string
		Email string
	}{
		Name:  name,
		Email: email,
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}
}

// func viewMessagesHandler(w http.ResponseWriter, r *http.Request) {
//     sender := r.URL.Query().Get("sender")
//     if sender == "" {
//         http.Error(w, "Sender not specified", http.StatusBadRequest)
//         return
//     }

//     adminEmail := "admin@123"
//     messages, err := fetchMessagesFromSender(adminEmail, sender)
//     if err != nil {
//         log.Printf("Failed to fetch messages: %v", err)
//         http.Error(w, "Internal Server Error", http.StatusInternalServerError)
//         return
//     }

//     tmpl, err := template.ParseFiles("static/viewmessages.html")
//     if err != nil {
//         log.Printf("Failed to parse template: %v", err)
//         http.Error(w, "Internal Server Error", http.StatusInternalServerError)
//         return
//     }

//     data := struct {
//         Messages []Message
//         Sender   string
//     }{
//         Messages: messages,
//         Sender:   sender,
//     }

//     err = tmpl.Execute(w, data)
//     if err != nil {
//         log.Printf("Failed to execute template: %v", err)
//         http.Error(w, "Internal Server Error", http.StatusInternalServerError)
//     }
// }

// func fetchMessagesFromSender(receiver string, sender string) ([]Message, error) {
//     var messages []Message
//     query := "SELECT sender, content, timestamp FROM messages WHERE receiver = $1 AND sender = $2"
//     rows, err := db.Query(query, receiver, sender)
//     if err != nil {
//         return nil, err
//     }
//     defer rows.Close()

//     for rows.Next() {
//         var msg Message
//         if err := rows.Scan(&msg.Sender, &msg.Content, &msg.Date); err != nil {
//             return nil, err
//         }
//         messages = append(messages, msg)
//     }
//     return messages, nil
// }

func sendReplyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	receiver := r.FormValue("receiver")
	sender := r.FormValue("sender")
	content := r.FormValue("content")

	if receiver == "" || sender == "" || content == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	query := "INSERT INTO messages (sender, receiver, content, date) VALUES ($1, $2, $3, NOW())"
	_, err := db.Exec(query, sender, receiver, content)
	if err != nil {
		log.Printf("Failed to insert reply: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	updateQuery := "UPDATE messages SET replied = TRUE WHERE sender = $1 AND receiver = $2 AND content = $3"
	_, err = db.Exec(updateQuery, receiver, sender, content)
	if err != nil {
		log.Printf("Failed to update message status: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/recruiter_dashboard", http.StatusSeeOther)
}

// func FetchMessages(email string) ([]Message, error) {
//     var messages []Message
//     query := "SELECT sender, content, timestamp FROM messages WHERE receiver = $1 AND replied = FALSE"
//     rows, err := db.Query(query, email)
//     if err != nil {
//         return nil, err
//     }
//     defer rows.Close()
//     for rows.Next() {
//         var msg Message
//         if err := rows.Scan(&msg.Sender, &msg.Content, &msg.Date); err != nil {
//             return nil, err
//         }
//         messages = append(messages, msg)
//     }
//     return messages, nil
// }

func candidatesHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("candidatesHandler: Start")

	cookie, err := r.Cookie("username")
	if err != nil {
		if err == http.ErrNoCookie {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		log.Printf("Error retrieving cookie: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	email := cookie.Value

	candidates, err := fetchAllCandidates()
	if err != nil {
		log.Printf("candidatesHandler: Error fetching candidates - %v\n", err)
		http.Error(w, "Unable to fetch candidates", http.StatusInternalServerError)
		return
	}
	log.Println("candidatesHandler: Candidates fetched successfully")

	data := struct {
		Candidates []Candidate
		Email      string
	}{
		Candidates: candidates,
		Email:      email,
	}

	tmpl, err := template.ParseFiles("static/candidates.html")
	if err != nil {
		log.Printf("candidatesHandler: Error loading template - %v\n", err)
		http.Error(w, "Unable to load template", http.StatusInternalServerError)
		return
	}
	log.Println("candidatesHandler: Template loaded successfully")

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Printf("candidatesHandler: Error executing template - %v\n", err)
		http.Error(w, "Unable to execute template", http.StatusInternalServerError)
		return
	}
	log.Println("candidatesHandler: Template executed successfully")
}

func fetchAllCandidates() ([]Candidate, error) {
	log.Println("fetchAllCandidates: Start")
	var candidates []Candidate

	rows, err := db.Query("SELECT name, email FROM candidate11")
	if err != nil {
		log.Printf("fetchAllCandidates: Error querying database - %v\n", err)
		return nil, err
	}
	defer rows.Close()
	log.Println("fetchAllCandidates: Query executed successfully")

	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.Name, &c.Email); err != nil {
			log.Printf("fetchAllCandidates: Error scanning row - %v\n", err)
			return nil, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		log.Printf("fetchAllCandidates: Error iterating rows - %v\n", err)
		return nil, err
	}
	log.Println("fetchAllCandidates: Candidates fetched successfully")
	return candidates, nil
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade the HTTP connection to a WebSocket connection
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Failed to upgrade connection:", err)
		http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Handle WebSocket communication
	for {
		messageType, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("Error while reading message:", err)
			break
		}

		// Log the received message (for debugging purposes)
		log.Printf("Received message: %s", msg)

		// Echo the received message back to the client
		if err := conn.WriteMessage(messageType, msg); err != nil {
			log.Println("Error while writing message:", err)
			break
		}
	}
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade:", err)
		return
	}
	defer conn.Close()

	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			log.Println("Read:", err)
			break
		}
		if err := conn.WriteMessage(messageType, p); err != nil {
			log.Println("Write:", err)
			break
		}
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	email := r.URL.Query().Get("email")
	client := &Client{conn: conn, email: email}
	clients[client] = true

	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("error: %v", err)
			delete(clients, client)
			break
		}
		msg.Sender = email
		msg.Timestamp = time.Now()
		broadcast <- msg
	}
}

func handleMessages(w http.ResponseWriter, r *http.Request) {
	var msg Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Ensure sender's email is validated and authenticated properly
	if msg.Sender == "" || msg.Recipient == "" || msg.Message == "" {
		http.Error(w, "Missing fields", http.StatusBadRequest)
		return
	}

	// Save message to the database with the provided sender
	_, err := db.Exec("INSERT INTO messages (sender, recipient, message) VALUES (?, ?, ?)", msg.Sender, msg.Recipient, msg.Message)
	if err != nil {
		http.Error(w, "Failed to save message", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getUserEmailFromDB(sessionID string) (string, error) {
	var email string
	err := db.QueryRow("SELECT email FROM users WHERE session_id = ?", sessionID).Scan(&email)
	if err != nil {
		return "", err
	}
	return email, nil
}

type User struct {
	Email string `json:"email"`
}

func checkAuthHandler(w http.ResponseWriter, r *http.Request) {

	session, err := store.Get(r, "session-name")
	if err != nil || session.Values["authenticated"] != true {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	sessionID, ok := session.Values["session_id"].(string)
	if !ok {
		http.Error(w, "Session ID not found", http.StatusInternalServerError)
		return
	}

	username, err := getUserEmailFromDB(sessionID)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"authenticated": true,
		"email":         username,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func fetchMessages(sender, recipient string) ([]Message, error) {
	rows, err := db.Query("SELECT sender, recipient, message FROM messages WHERE (sender = ? AND recipient = ?) OR (sender = ? AND recipient = ?)", sender, recipient, recipient, sender)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.Sender, &msg.Recipient, &msg.Message); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func saveMessageToDB(msg Message) error {
	query := `
        INSERT INTO messages (sender, recipient, message, timestamp)
        VALUES ($1, $2, $3, $4)
    `
	log.Printf(query)
	_, err := db.Exec(query, msg.Sender, msg.Recipient, msg.Message, msg.Timestamp)
	if err != nil {
		log.Printf("Could not save message: %v", err)
		return err
	}
	return nil
}

func saveMessageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"status": "error", "message": "Invalid request method"}`, http.StatusMethodNotAllowed)
		return
	}

	var message struct {
		Sender    string `json:"sender"`
		Recipient string `json:"recipient"`
		Message   string `json:"message"`
		Timestamp string `json:"timestamp"`
	}

	if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
		http.Error(w, `{"status": "error", "message": "Invalid request body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	query := `INSERT INTO messages (sender, recipient, message, timestamp) VALUES ($1, $2, $3, $4)`
	_, err := db.Exec(query, message.Sender, message.Recipient, message.Message, message.Timestamp)
	if err != nil {
		log.Printf("Failed to save message: %v", err)
		http.Error(w, `{"status": "error", "message": "Failed to save message"}`, http.StatusInternalServerError)
		return
	}

	response := map[string]string{"status": "success", "message": "Message saved"}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func saveMessagesHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, `{"status": "error", "message": "Invalid request method"}`, http.StatusMethodNotAllowed)
		return
	}

	var message Message
	if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
		http.Error(w, `{"status": "error", "message": "Invalid request body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	query := `INSERT INTO messages (sender, recipient, message, timestamp) VALUES ($1, $2, $3, $4)`
	_, err := db.Exec(query, message.Sender, message.Recipient, message.Message, message.Timestamp)
	if err != nil {
		log.Printf("Failed to save message: %v", err)
		http.Error(w, `{"status": "error", "message": "Failed to save message"}`, http.StatusInternalServerError)
		return
	}

	response := map[string]string{"status": "success", "message": "Message saved"}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// func FetchChatHistoryHandler(w http.ResponseWriter, r *http.Request) {
// 	// Get user session
// 	session, err := store.Get(r, "session-name")
// 	if err != nil {
// 		http.Error(w, "Unable to retrieve session", http.StatusInternalServerError)
// 		return
// 	}

// 	// Retrieve the current user's email from the session
// 	senderEmail, ok := session.Values["email"].(string)
// 	if !ok || senderEmail == "" {
// 		http.Error(w, "User not logged in", http.StatusUnauthorized)
// 		return
// 	}

// 	recipientEmail := r.URL.Query().Get("email")
// 	if recipientEmail == "" {
// 		http.Error(w, "Recipient email not provided", http.StatusBadRequest)
// 		return
// 	}

// 	// Fetch chat history from database
// 	messages, err := fetchChatHistory(senderEmail, recipientEmail)
// 	if err != nil {
// 		http.Error(w, "Unable to fetch chat history", http.StatusInternalServerError)
// 		return
// 	}

// 	// Prepare response
// 	response := make([]Message, len(messages))
// 	for i, msg := range messages {
// 		response[i] = Message{
// 			Sender:    msg.Sender,
// 			Message:   msg.Message,
// 			Timestamp: msg.Timestamp,
// 		}
// 	}

// 	w.Header().Set("Content-Type", "application/json")
// 	json.NewEncoder(w).Encode(response)
// }

// fetchChatHistory retrieves messages between sender and recipient from the database
func fetchChatHistory(senderEmail, recipientEmail string) ([]Message, error) {
	// Replace with your actual database connection setup
	db, err := sql.Open("mysql", "user:password@tcp(localhost:3306)/yourdatabase")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// Query to fetch chat history
	query := `SELECT sender, message, sent_time FROM messages 
              WHERE (sender = ? AND recipient = ?) 
              OR (sender = ? AND recipient = ?) 
              ORDER BY timestamp ASC`
	rows, err := db.Query(query, senderEmail, recipientEmail, recipientEmail, senderEmail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.Sender, &msg.Message, &msg.Timestamp); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

func handleGetPreviousMessages(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	messages, ok := messageHistory[email]
	if !ok {
		messages = []Message{}
	}

	response := struct {
		Messages []Message `json:"messages"`
	}{
		Messages: messages,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// func handleWebSocket(w http.ResponseWriter, r *http.Request) {
//     email := r.URL.Query().Get("email")

//     conn, err := upgrader.Upgrade(w, r, nil)
//     if err != nil {
//         log.Println("Error while upgrading connection:", err)
//         return
//     }
//     defer conn.Close()

//     client := &Client{
//         Email: email,
//         Conn:  conn,
//         Send:  make(chan Message),
//     }

//     clients.Lock()
//     clients.clients[email] = client
//     clients.Unlock()

//     log.Printf("WebSocket connection established with %s", email)

//     go handleMessages(client)

//     for {
//         _, msgBytes, err := conn.ReadMessage()
//         if err != nil {
//             log.Println("Error while reading message:", err)
//             break
//         }

//         var msg Message
//         if err := json.Unmarshal(msgBytes, &msg); err != nil {
//             log.Println("Error while unmarshalling message:", err)
//             continue
//         }

//         log.Printf("Received message from %s: %s", email, msg.Content)

//         clients.Lock()
//         for _, c := range clients.clients {
//             if c.Email != email {
//                 err := c.Conn.WriteMessage(websocket.TextMessage, msgBytes)
//                 if err != nil {
//                     log.Println("Error while writing message:", err)
//                 }
//             }
//         }
//         clients.Unlock()
//     }

//     clients.Lock()
//     delete(clients.clients, email)
//     clients.Unlock()
//     log.Printf("WebSocket connection closed with %s", email)
// }

func getUserEmail(w http.ResponseWriter, r *http.Request) {

	userID := r.URL.Query().Get("candidate_id")
	if userID == "" {
		http.Error(w, "Missing user_id parameter", http.StatusBadRequest)
		return
	}

	connStr := "user=postgres dbname=candidate_db sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var email string
	query := "SELECT email FROM candidate11 WHERE id=$1"
	err = db.QueryRow(query, userID).Scan(&email)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "No user found with the given ID", http.StatusNotFound)
		} else {
			log.Printf("Database query error: %v", err)
			http.Error(w, "Database query error", http.StatusInternalServerError)
		}
		return
	}

	response := map[string]string{"email": email}
	json.NewEncoder(w).Encode(response)
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	log.Printf("Handling chat for user: %s\n", email)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error while upgrading connection for user %s: %v\n", email, err)
		http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
		return
	}
	log.Printf("Connection upgraded for user %s\n", email)
	defer conn.Close()

	for {
		messageType, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error while reading message from user %s: %v\n", email, err)
			break
		}
		log.Printf("Received message from user %s: %s\n", email, msg)

		// Broadcast the received message to all clients
		broadcastMessage(email, msg)
		log.Printf("Broadcasted message from user %s\n", email)

		// Send the message back to the client
		if err := conn.WriteMessage(messageType, msg); err != nil {
			log.Printf("Error while writing message back to user %s: %v\n", email, err)
			break
		}
		log.Printf("Sent message back to user %s\n", email)
	}
}

func broadcastMessage(email string, msg []byte) {

	messages := messageHistory[email]
	messages = append(messages, Message{Sender: "You", Message: string(msg)})
	messageHistory[email] = messages
}

// func handleMessages() {
//     for {
//         msg := <-broadcast
//         for client := range clients {
//             err := client.WriteJSON(msg)
//             if err != nil {
//                 log.Printf("error: %v", err)
//                 client.Close()
//                 delete(clients, client)
//             }
//         }
//     }
// }

// func authMiddleware(next http.Handler) http.Handler {
//     return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//         user, err := getUserFromSession(r)
//         if err != nil || user == "" {
//             log.Printf("Auth middleware error: %v", err)
//             http.Redirect(w, r, "/login", http.StatusSeeOther)
//             return
//         }

//         log.Printf("Authenticated user: %s", user)

//         ctx := context.WithValue(r.Context(), "user", user)
//         next.ServeHTTP(w, r.WithContext(ctx))
//     })
// }

// func getUserFromSession(r *http.Request) (string, error) {
//     session, err := store.Get(r, "session-name")
//     if err != nil {
//         log.Printf("Session retrieval error: %v", err)
//         return "", err
//     }

//     user, ok := session.Values["user"].(string)
//     if !ok {
//         log.Printf("User not found in session")
//         return "", errors.New("user not found in session")
//     }

//     return user, nil
// }

func fetchChatHistoryHandler(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "Missing email parameter", http.StatusBadRequest)
		return
	}

	rows, err := db.Query("SELECT sender, recipient, message, timestamp FROM messages WHERE sender = $1 OR recipient = $1 ORDER BY timestamp ASC", email)
	if err != nil {
		http.Error(w, "Database query error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var messages []map[string]interface{}
	for rows.Next() {
		var sender, recipient, message string
		var timestamp string
		if err := rows.Scan(&sender, &recipient, &message, &timestamp); err != nil {
			http.Error(w, "Database scan error", http.StatusInternalServerError)
			return
		}
		messages = append(messages, map[string]interface{}{
			"sender":    sender,
			"recipient": recipient,
			"message":   message,
			"timestamp": timestamp,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func getUserEmailHandler(w http.ResponseWriter, r *http.Request) {
	session, err := store.Get(r, "session-name")
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	userID, ok := session.Values["user_id"].(string)
	if !ok || userID == "" {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	var username string
	err = db.QueryRow("SELECT email FROM users WHERE id = $1", userID).Scan(&username)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "User not found", http.StatusNotFound)
		} else {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	response := map[string]string{"email": username}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func getMessagesHandler(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, `{"error": "Email parameter is missing"}`, http.StatusBadRequest)
		return
	}

	rows, err := db.Query(`
		SELECT sender, recipient, message, timestamp 
		FROM messages 
		WHERE sender = $1 OR recipient = $1
		ORDER BY timestamp ASC
	`, email)
	if err != nil {
		http.Error(w, `{"error": "Error fetching messages"}`, http.StatusInternalServerError)
		log.Println("Error fetching messages:", err)
		return
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.Sender, &msg.Recipient, &msg.Message, &msg.Timestamp); err != nil {
			http.Error(w, `{"error": "Error reading message"}`, http.StatusInternalServerError)
			log.Println("Error reading message:", err)
			return
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, `{"error": "Error with rows iteration"}`, http.StatusInternalServerError)
		log.Println("Error with rows iteration:", err)
		return
	}

	response := struct {
		Messages []Message `json:"messages"`
	}{
		Messages: messages,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, `{"error": "Error encoding JSON"}`, http.StatusInternalServerError)
		log.Println("Error encoding JSON:", err)
	}
}

func handleWebSocketConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error while upgrading connection:", err)
		return
	}
	defer conn.Close()

	client := &Client{conn: conn}
	clients[client] = true

	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Println("Error while reading message:", err)
			break
		}

		// Broadcast the message to all clients
		broadcast <- msg
	}

	// Remove the client from the map when done
	delete(clients, client)
}

func handleBroadcastMessages() {
	for msg := range broadcast {
		// Broadcast the message to all connected clients
		for client := range clients {
			if err := client.conn.WriteJSON(msg); err != nil {
				log.Println("Error while broadcasting message:", err)
				client.conn.Close()
				delete(clients, client)
			}
		}
	}
}
