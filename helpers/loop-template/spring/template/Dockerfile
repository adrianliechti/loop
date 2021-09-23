FROM maven:3-openjdk-11-slim as builder

WORKDIR /src

COPY pom.xml .
RUN mvn dependency:go-offline --no-transfer-progress

COPY . .
RUN mvn clean install -DskipTests --no-transfer-progress


FROM openjdk:11-jre-slim-buster

RUN apt-get update -qq && apt-get install -qq -y --no-install-recommends \
    tini \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

RUN groupadd -r app -g 1000 \
 && useradd -m -r app -u 1000 -g app -s /usr/sbin/nologin \
 && chown app:app /app

COPY --from=builder --chown=app:app /src/target/${APP_ARTIFACT}-*.jar /app/${APP_ARTIFACT}.jar

USER 1000

ENTRYPOINT ["tini", "--"]
CMD ["java", "-jar", "/app/${APP_ARTIFACT}.jar"]