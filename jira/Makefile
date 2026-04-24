.PHONY: all clean

all: create-jira-ticket

create-jira-ticket: create-jira-ticket.go slack.go
	go build -o create-jira-ticket create-jira-ticket.go slack.go

clean:
	rm -f create-jira-ticket
