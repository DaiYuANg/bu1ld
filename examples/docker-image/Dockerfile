FROM alpine:3.20

ARG MESSAGE="hello"
RUN printf '%s\n' "$MESSAGE" > /message.txt
CMD ["cat", "/message.txt"]
