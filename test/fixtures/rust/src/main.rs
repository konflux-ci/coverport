use axum::{extract::Query, response::IntoResponse, routing::get, Router};
use std::collections::HashMap;

fn greet(name: &str) -> String {
    if name.is_empty() {
        "Hello, World!".to_string()
    } else if name.eq_ignore_ascii_case("coverport") {
        "Hello from the CoverPort test fixture!".to_string()
    } else {
        format!("Hello, {name}!")
    }
}

async fn hello_handler(Query(params): Query<HashMap<String, String>>) -> impl IntoResponse {
    let name = params.get("name").map(|s| s.as_str()).unwrap_or("");
    let msg = greet(name);
    format!("{msg}\n")
}

#[tokio::main]
async fn main() {
    let _handle = coverage_server::start_coverage_server_standalone(53700);

    let app = Router::new().route("/hello", get(hello_handler));

    let port: u16 = std::env::var("APP_PORT")
        .ok()
        .and_then(|p| p.parse().ok())
        .unwrap_or(8080);

    let listener = tokio::net::TcpListener::bind(format!("0.0.0.0:{port}"))
        .await
        .expect("failed to bind app port");

    println!("Test fixture app listening on :{port}");
    axum::serve(listener, app).await.unwrap();
}
