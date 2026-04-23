.PHONY: all clean

all: create-jira-ticket

create-jira-ticket: create-jira-ticket.go
	go build -o create-jira-ticket create-jira-ticket.go

clean:
	rm -f create-jira-ticket
