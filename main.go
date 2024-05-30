package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	hub string
	dr1 string
	dr2 string
)

func init() {
	flag.StringVar(&hub, "hub", "", "path to hub kubeconfig")
	flag.StringVar(&dr1, "dr1", "", "path to dr1 kubeconfig")
	flag.StringVar(&dr2, "dr2", "", "path to dr2 kubeconfig")
	flag.Parse()
}

type clusterInfo struct {
	name           string
	status         string
	context        string
	namespaces     []string
	hub            bool
	managedcluster bool
}

type model struct {
	clusters []clusterInfo
	cursor   int
	width    int
	height   int
	ticker   *time.Ticker
}

func initialModel() model {
	return model{
		clusters: []clusterInfo{
			{"Hub", "Unknown", hub, []string{}, true, false},
			{"DR1", "Unknown", dr1, []string{}, false, true},
			{"DR2", "Unknown", dr2, []string{}, false, true},
		},
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchClustersData(&m.clusters), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return t
	})
}

func fetchClusterClient(kubeconfig string) *kubernetes.Clientset {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil
	}

	return clientset
}

func updateClusterData(c *clusterInfo) {
	client := fetchClusterClient(c.context)
	if client == nil {
		c.status = "Error"
		c.namespaces = []string{}
	}

	c.status = getClusterStatus(client)
	c.namespaces = getRamenNamespaces(client)
}

func fetchClustersData(clusters *[]clusterInfo) tea.Cmd {
	return func() tea.Msg {
		for i := range *clusters {
			updateClusterData(&(*clusters)[i])
		}
		return clusters
	}
}

func filterRamenNamespaces(namespaces *v1.NamespaceList) []string {
	var filtered []string
	for _, ns := range namespaces.Items {
		if ns.Name == "ramen-system" ||
			ns.Name == "ramen-ops" ||
			ns.Name == "openshift-operators" ||
			ns.Name == "openshift-dr-system" {
			filtered = append(filtered, ns.Name)
		}
	}
	return filtered
}

func getNamespaces(clientset *kubernetes.Clientset) *v1.NamespaceList {
	namespaces, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return &v1.NamespaceList{}
	}

	return namespaces
}

func getRamenNamespaces(clientset *kubernetes.Clientset) []string {
	ns := getNamespaces(clientset)
	return filterRamenNamespaces(ns)
}

func getClusterStatus(clientset *kubernetes.Clientset) string {
	_, err := clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "Error"
	}

	return "Healthy"
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case []clusterInfo:
		m.clusters = msg

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.clusters)-1 {
				m.cursor++
			}
		}
	case time.Time:
		return m, tea.Batch(fetchClustersData(&m.clusters), tickCmd())
	}
	return m, nil
}

func getHubStyle(cluster *clusterInfo, width, height int) string {
	var msg []string
	msg = append(msg, fmt.Sprintf("%s\n\n", cluster.name))
	msg = append(msg, fmt.Sprintf("Status: %s\n", cluster.status))
	msg = append(msg, fmt.Sprintf("Namespaces: %s\n", cluster.namespaces))
	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true).
		Width(width).
		Height(height).
		Align(lipgloss.Left).
		SetString(msg...)

	return style.Render()
}

func getManagedClusterStyle(cluster *clusterInfo, width, height int, align string) string {
	var msg []string
	msg = append(msg, fmt.Sprintf("%s\n\n", cluster.name))
	msg = append(msg, fmt.Sprintf("Status: %s\n", cluster.status))
	msg = append(msg, fmt.Sprintf("Namespaces: %s\n", cluster.namespaces))
	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true).
		Width(width).
		Height(height).
		Align(lipgloss.Left).
		SetString(msg...)

	return style.Render()
}

func (m model) View() string {
	var hubStyle string
	var dr1Style string
	var dr2Style string

	hubStyle = getHubStyle(&m.clusters[0], m.width, m.height/3)
	dr1Style = getManagedClusterStyle(&m.clusters[1], m.width/2, m.height/2, "left")
	dr2Style = getManagedClusterStyle(&m.clusters[2], m.width/2, m.height/2, "right")

	// Render the final view with the new layout
	s := lipgloss.JoinVertical(
		lipgloss.Top,
		hubStyle,
		lipgloss.JoinHorizontal(
			lipgloss.Left,
			dr1Style,
			dr2Style))
	s += "\n\nPress q to quit.\n"

	return s
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
