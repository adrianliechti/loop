FROM mcr.microsoft.com/dotnet/core/sdk:3.1 as builder

WORKDIR /src

COPY *.csproj .
RUN dotnet restore

COPY . .
RUN dotnet publish -c Release -o /app --nologo --no-restore --no-self-contained


FROM mcr.microsoft.com/dotnet/core/aspnet:3.1

RUN apt-get update -qq && apt-get install -qq -y --no-install-recommends \
    tini \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

RUN groupadd -r app -g 1000 \
 && useradd -m -r app -u 1000 -g app -s /usr/sbin/nologin \
 && chown app:app /app

COPY --from=builder --chown=app:app /app /app

USER 1000

ENV ASPNETCORE_URLS=http://+:8080

ENTRYPOINT ["tini", "--"]
CMD ["dotnet", "/app/${APP_NAME}.dll"]