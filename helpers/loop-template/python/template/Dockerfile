FROM python:3.9-alpine

RUN apk add --no-cache ca-certificates tini

WORKDIR /app

COPY requirements.txt requirements.txt
RUN pip3 install -r requirements.txt

COPY . .

RUN addgroup -g 1000 -S app \
 && adduser -u 1000 -S -G app app \
 && chown -R app:app /app

USER 1000

ENTRYPOINT ["/sbin/tini", "--"]
CMD [ "python3", "-m" , "flask", "run", "--host=0.0.0.0"]