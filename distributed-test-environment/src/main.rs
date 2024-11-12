// main.rs
use tokio::{net::TcpListener, io::{AsyncReadExt, AsyncWriteExt}};
use hyper::{Server, Response, Body, Request, StatusCode};
use std::convert::Infallible;
use std::sync::Arc;
use tokio::sync::RwLock;
use parking_lot::RwLock as PLRwLock;
use mimalloc::MiMalloc;
use futures_util::future::join_all;

#[global_allocator]
static GLOBAL: MiMalloc = MiMalloc;

// Custom memory-efficient router
struct Router {
    routes: PLRwLock<Vec<(String, Arc<dyn Fn(Request<Body>) -> Response<Body> + Send + Sync>)>>
}

impl Router {
    fn new() -> Self {
        Router {
            routes: PLRwLock::new(Vec::with_capacity(16))
        }
    }

    fn add_route<F>(&self, path: &str, handler: F)
    where
        F: Fn(Request<Body>) -> Response<Body> + Send + Sync + 'static
    {
        self.routes.write().push((path.to_string(), Arc::new(handler)));
    }
}

async fn handle_request(req: Request<Body>, router: Arc<Router>) -> Result<Response<Body>, Infallible> {
    let path = req.uri().path().to_string();
    let routes = router.routes.read();
    
    for (route_path, handler) in routes.iter() {
        if path == *route_path {
            return Ok(handler(req));
        }
    }
    
    Ok(Response::builder()
        .status(StatusCode::NOT_FOUND)
        .body(Body::from("Not Found"))
        .unwrap())
}

#[tokio::main]
async fn main() {
    // Configure server
    let addr = "0.0.0.0:8080";
    let router = Arc::new(Router::new());
    
    // Setup routes
    let router_clone = router.clone();
    router.add_route("/", move |_req| {
        Response::new(Body::from("Hello from Rust!"))
    });
    
    // Health check endpoint
    router.add_route("/health", |_req| {
        Response::new(Body::from("OK"))
    });

    // Create service
    let make_service = hyper::service::make_service_fn(move |_| {
        let router = router_clone.clone();
        async move {
            Ok::<_, Infallible>(hyper::service::service_fn(move |req| {
                handle_request(req, router.clone())
            }))
        }
    });

    // Start server with optimized settings
    let server = Server::bind(&addr.parse().unwrap())
        .http1_keepalive(true)
        .http1_half_close(false)
        .tcp_nodelay(true)
        .serve(make_service);

    println!("Server running on {}", addr);
    
    if let Err(e) = server.await {
        eprintln!("Server error: {}", e);
    }
}
